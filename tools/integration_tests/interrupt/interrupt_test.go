// Copyright 2024 Google LLC
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

package interrupt

import (
	"context"
	"log"
	"os"
	"path"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/client"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/mounting/static_mounting"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/setup"
)

const (
	testDirName = "InterruptTest"
)

////////////////////////////////////////////////////////////////////////
// TestMain
////////////////////////////////////////////////////////////////////////

func TestMain(m *testing.M) {
	setup.ParseSetUpFlags()

	var storageClient *storage.Client
	ctx := context.Background()
	closeStorageClient := client.CreateStorageClientWithCancel(&ctx, &storageClient)
	defer func() {
		err := closeStorageClient()
		if err != nil {
			log.Fatalf("closeStorageClient failed: %v", err)
		}
	}()

	setup.RunTestsForMountedDirectoryFlag(m)

	// Else run tests for testBucket.
	// Set up test directory.
	setup.SetUpTestDirForTestBucketFlag()

	// Set up flags to run tests on.
	yamlContent1 := map[string]interface{}{
		"file-system": map[string]interface{}{
			"ignore-interrupts": true,
		},
	}
	yamlContent2 := map[string]interface{}{} // test default
	flags := [][]string{
		{"--implicit-dirs=true", "--enable-streaming-writes=false"},
		{"--config-file=" + setup.YAMLConfigFile(yamlContent1, "ignore_interrupts.yaml"), "--enable-streaming-writes=false"},
		{"--config-file=" + setup.YAMLConfigFile(yamlContent2, "default_ignore_interrupts.yaml"), "--enable-streaming-writes=false"}}
	// TODO(b/417136852): Enable this test for Zonal Bucket also once read start working.
	if !setup.IsZonalBucketRun() {
		flags = append(flags, []string{"--enable-streaming-writes=true"})
	}
	successCode := static_mounting.RunTests(flags, m)

	// Clean up test directory created.
	setup.CleanupDirectoryOnGCS(ctx, storageClient, path.Join(setup.TestBucket(), testDirName))
	os.Exit(successCode)
}
