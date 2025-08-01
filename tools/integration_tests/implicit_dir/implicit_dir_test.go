// Copyright 2023 Google LLC
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

// Provide tests when implicit directory present and mounted bucket with --implicit-dir flag.
package implicit_dir_test

import (
	"context"
	"log"
	"os"
	"path"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/client"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/setup"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/setup/implicit_and_explicit_dir_setup"
)

const ExplicitDirInImplicitDir = "explicitDirInImplicitDir"
const ExplicitDirInImplicitSubDir = "explicitDirInImplicitSubDir"
const PrefixFileInExplicitDirInImplicitDir = "fileInExplicitDirInImplicitDir"
const PrefixFileInExplicitDirInImplicitSubDir = "fileInExplicitDirInImplicitSubDir"
const NumberOfFilesInExplicitDirInImplicitSubDir = 1
const NumberOfFilesInExplicitDirInImplicitDir = 1
const DirForImplicitDirTests = "dirForImplicitDirTests"

// IMPORTANT: To prevent global variable pollution, enhance code clarity,
// and avoid inadvertent errors. We strongly suggest that, all new package-level
// variables (which would otherwise be declared with `var` at the package root) should
// be added as fields to this 'env' struct instead.
type env struct {
	storageClient *storage.Client
	ctx           context.Context
	testDirPath   string
}

var testEnv env

func setupTestDir(dirName string) string {
	dir := setup.SetupTestDirectory(DirForImplicitDirTests)
	dirPath := path.Join(dir, dirName)
	err := os.Mkdir(dirPath, setup.DirPermission_0755)
	if err != nil {
		log.Fatalf("Error while setting up directory %s for testing: %v", dirPath, err)
	}

	return dirPath
}
func TestMain(m *testing.M) {
	setup.ParseSetUpFlags()

	// Create storage client before running tests.
	testEnv.ctx = context.Background()
	closeStorageClient := client.CreateStorageClientWithCancel(&testEnv.ctx, &testEnv.storageClient)
	defer func() {
		err := closeStorageClient()
		if err != nil {
			log.Fatalf("closeStorageClient failed: %v", err)
		}
	}()

	flagsSet := [][]string{{"--implicit-dirs"}}

	// No need to run enable-hns and client-protocol GRPC configuration for ZB,
	// as those are both by default enabled for ZB.
	if !setup.IsZonalBucketRun() {
		if hnsFlagSet, err := setup.AddHNSFlagForHierarchicalBucket(testEnv.ctx, testEnv.storageClient); err == nil {
			flagsSet = append(flagsSet, hnsFlagSet)
		}

		if !testing.Short() {
			flagsSet = append(flagsSet, []string{"--client-protocol=grpc", "--implicit-dirs"})
		}
	}

	successCode := implicit_and_explicit_dir_setup.RunTestsForImplicitDirAndExplicitDir(flagsSet, m)
	setup.SaveLogFileInCaseOfFailure(successCode)

	// Clean up test directory created.
	setup.CleanupDirectoryOnGCS(testEnv.ctx, testEnv.storageClient, path.Join(setup.TestBucket(), testDirName))
	os.Exit(successCode)
}
