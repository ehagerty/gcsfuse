// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gcsx

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/googlecloudplatform/gcsfuse/v3/cfg"
	"github.com/googlecloudplatform/gcsfuse/v3/internal/clock"
	"github.com/googlecloudplatform/gcsfuse/v3/internal/logger"
	"github.com/googlecloudplatform/gcsfuse/v3/internal/monitor"
	"github.com/googlecloudplatform/gcsfuse/v3/internal/storage/gcs"
	"github.com/googlecloudplatform/gcsfuse/v3/metrics"
	"golang.org/x/net/context"
)

// Timeout value which determines when the MultiRangeDownloader will be closed after
// it's refcount reaches 0.
const multiRangeDownloaderTimeout = 60 * time.Second

func NewMultiRangeDownloaderWrapper(bucket gcs.Bucket, object *gcs.MinObject, config *cfg.Config) (MultiRangeDownloaderWrapper, error) {
	return NewMultiRangeDownloaderWrapperWithClock(bucket, object, clock.RealClock{}, config)
}

func NewMultiRangeDownloaderWrapperWithClock(bucket gcs.Bucket, object *gcs.MinObject, clock clock.Clock, config *cfg.Config) (MultiRangeDownloaderWrapper, error) {
	if object == nil {
		return MultiRangeDownloaderWrapper{}, fmt.Errorf("NewMultiRangeDownloaderWrapperWithClock: Missing MinObject")
	}
	// In case of a local inode, MRDWrapper would be created with an empty minObject (i.e. with a minObject without any information)
	// and when the object is actually created, MRDWrapper would be updated using SetMinObject method.
	return MultiRangeDownloaderWrapper{
		clock:  clock,
		bucket: bucket,
		object: object,
		config: config,
	}, nil
}

type readResult struct {
	bytesRead int
	err       error
}

type MultiRangeDownloaderWrapper struct {
	// Holds the object implementing MultiRangeDownloader interface.
	Wrapped gcs.MultiRangeDownloader

	// Bucket and object details for MultiRangeDownloader.
	// Object should not be nil.
	object *gcs.MinObject
	bucket gcs.Bucket

	// Refcount is used to determine when to close the MultiRangeDownloader.
	refCount int
	// Mutex is used to synchronize access over refCount.
	mu sync.RWMutex
	// Holds the cancel function, which can be called to cancel the cleanup function.
	cancelCleanup context.CancelFunc
	// Used for waiting for timeout (helps us in mocking the functionality).
	clock clock.Clock
	// GCSFuse mount config.
	config *cfg.Config
}

// SetMinObject sets the gcs.MinObject stored in the wrapper to passed value, only if it's non nil.
func (mrdWrapper *MultiRangeDownloaderWrapper) SetMinObject(minObj *gcs.MinObject) error {
	if minObj == nil {
		return fmt.Errorf("MultiRangeDownloaderWrapper::SetMinObject: Missing MinObject")
	}
	mrdWrapper.object = minObj
	return nil
}

// GetMinObject returns the minObject stored in MultiRangeDownloaderWrapper. Used only for unit testing.
func (mrdWrapper *MultiRangeDownloaderWrapper) GetMinObject() *gcs.MinObject {
	return mrdWrapper.object
}

// GetRefCount returns current refcount.
func (mrdWrapper *MultiRangeDownloaderWrapper) GetRefCount() int {
	mrdWrapper.mu.RLock()
	defer mrdWrapper.mu.RUnlock()
	return mrdWrapper.refCount
}

// IncrementRefCount increments the refcount and cancel any running cleanup function.
// This method should be called exactly once per user of this wrapper.
// It has to be called before using the MultiRangeDownloader.
func (mrdWrapper *MultiRangeDownloaderWrapper) IncrementRefCount() {
	mrdWrapper.mu.Lock()
	defer mrdWrapper.mu.Unlock()

	mrdWrapper.refCount++
	if mrdWrapper.cancelCleanup != nil {
		mrdWrapper.cancelCleanup()
		mrdWrapper.cancelCleanup = nil
	}
}

// DecrementRefCount decrements the refcount. In case refcount reaches 0, cleanup the MRD.
// Returns error on invalid usage.
// This method should be called exactly once per user of this wrapper
// when MultiRangeDownloader is no longer needed & can be cleaned up.
func (mrdWrapper *MultiRangeDownloaderWrapper) DecrementRefCount() (err error) {
	mrdWrapper.mu.Lock()
	defer mrdWrapper.mu.Unlock()

	if mrdWrapper.refCount <= 0 {
		err = fmt.Errorf("MultiRangeDownloaderWrapper DecrementRefCount: Refcount cannot be negative")
		return
	}

	mrdWrapper.refCount--
	if mrdWrapper.refCount == 0 && mrdWrapper.Wrapped != nil {
		mrdWrapper.Wrapped.Close()
		mrdWrapper.Wrapped = nil
		// TODO (b/391508479): Start using cleanup function when MRD recreation is handled
		// mrdWrapper.cleanupMultiRangeDownloader()
	}
	return
}

// Spawns a cancellable go routine to close the MRD after the timeout.
// Always call after taking MultiRangeDownloaderWrapper's mutex lock.
func (mrdWrapper *MultiRangeDownloaderWrapper) cleanupMultiRangeDownloader() {
	closeMRD := func(ctx context.Context) {
		select {
		case <-mrdWrapper.clock.After(multiRangeDownloaderTimeout):
			mrdWrapper.mu.Lock()
			defer mrdWrapper.mu.Unlock()

			if mrdWrapper.refCount == 0 && mrdWrapper.Wrapped != nil {
				mrdWrapper.Wrapped.Close()
				mrdWrapper.Wrapped = nil
				mrdWrapper.cancelCleanup = nil
			}
		case <-ctx.Done():
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	mrdWrapper.cancelCleanup = cancel
	go closeMRD(ctx)
}

// Ensures that MultiRangeDownloader exists, creating it if it does not exist.
func (mrdWrapper *MultiRangeDownloaderWrapper) ensureMultiRangeDownloader() (err error) {
	if mrdWrapper.object == nil || mrdWrapper.bucket == nil {
		return fmt.Errorf("ensureMultiRangeDownloader error: Missing minObject or bucket")
	}

	mrdWrapper.mu.RLock()
	// Create the MRD if it does not exist.
	// In case the existing MRD is unusable due to closed stream, recreate the MRD.
	if mrdWrapper.Wrapped == nil || mrdWrapper.Wrapped.Error() != nil {
		mrdWrapper.mu.RUnlock()
		mrdWrapper.mu.Lock()
		defer mrdWrapper.mu.Unlock()

		// Checking if the mrdWrapper state is same after taking the lock.
		if mrdWrapper.Wrapped == nil || mrdWrapper.Wrapped.Error() != nil {
			var mrd gcs.MultiRangeDownloader
			mrd, err = mrdWrapper.bucket.NewMultiRangeDownloader(context.Background(), &gcs.MultiRangeDownloaderRequest{
				Name:           mrdWrapper.object.Name,
				Generation:     mrdWrapper.object.Generation,
				ReadCompressed: mrdWrapper.object.HasContentEncodingGzip(),
			})
			if err == nil {
				// Updating mrdWrapper.Wrapped only when MRD creation was successful.
				mrdWrapper.Wrapped = mrd
			}
		}
	} else {
		mrdWrapper.mu.RUnlock()
	}
	return
}

// Reads the data using MultiRangeDownloader.
func (mrdWrapper *MultiRangeDownloaderWrapper) Read(ctx context.Context, buf []byte,
	startOffset int64, endOffset int64, timeout time.Duration, metricHandle metrics.MetricHandle) (bytesRead int, err error) {
	// Bidi Api with 0 as read_limit means no limit whereas we do not want to read anything with empty buffer.
	// Hence, handling it separately.
	if len(buf) == 0 {
		return 0, nil
	}

	err = mrdWrapper.ensureMultiRangeDownloader()
	if err != nil {
		err = fmt.Errorf("MultiRangeDownloaderWrapper::Read: Error in creating MultiRangeDownloader:  %v", err)
		return
	}

	// We will only read what is requested by the client. Hence, capping end to the requested value.
	if endOffset > startOffset+int64(len(buf)) {
		endOffset = startOffset + int64(len(buf))
	}

	buffer := bytes.NewBuffer(buf)
	buffer.Reset()
	done := make(chan readResult, 1)

	mu := sync.Mutex{}
	defer func() {
		mu.Lock()
		close(done)
		done = nil
		mu.Unlock()
	}()

	start := time.Now()
	mrdWrapper.Wrapped.Add(buffer, startOffset, endOffset-startOffset, func(offsetAddCallback int64, bytesReadAddCallback int64, e error) {
		defer func() {
			mu.Lock()
			if done != nil {
				done <- readResult{bytesRead: int(bytesReadAddCallback), err: e}
			}
			mu.Unlock()
		}()

		if e != nil && e != io.EOF {
			e = fmt.Errorf("error in Add call: %w", e)
		}
	})

	if !mrdWrapper.config.FileSystem.IgnoreInterrupts {
		select {
		case <-time.After(timeout):
			err = fmt.Errorf("timeout")
		case <-ctx.Done():
			err = ctx.Err()
		case res := <-done:
			bytesRead = res.bytesRead
			err = res.err
		}
	} else {
		res := <-done
		bytesRead = res.bytesRead
		err = res.err
	}
	if err != nil {
		err = fmt.Errorf("MultiRangeDownloaderWrapper::Read: %w", err)
		logger.Error(err.Error())
	}
	monitor.CaptureMultiRangeDownloaderMetrics(ctx, metricHandle, "MultiRangeDownloader::Add", start)

	return
}
