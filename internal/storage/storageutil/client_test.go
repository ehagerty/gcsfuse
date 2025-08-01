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

package storageutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var keyFile = "testdata/key.json"

func TestClient(t *testing.T) {
	suite.Run(t, new(clientTest))
}

type clientTest struct {
	suite.Suite
}

// Tests

func (t *clientTest) TestCreateHttpClientWithHttp1() {
	sc := GetDefaultStorageClientConfig(keyFile) // By default http1 enabled

	httpClient, err := CreateHttpClient(&sc, nil)

	assert.NoError(t.T(), err)
	assert.NotNil(t.T(), httpClient)
	assert.Equal(t.T(), sc.HttpClientTimeout, httpClient.Timeout)
}

func (t *clientTest) TestCreateHttpClientWithHttp2() {
	sc := GetDefaultStorageClientConfig(keyFile)

	httpClient, err := CreateHttpClient(&sc, nil)

	assert.NoError(t.T(), err)
	assert.NotNil(t.T(), httpClient)
	assert.Equal(t.T(), sc.HttpClientTimeout, httpClient.Timeout)
}

func (t *clientTest) TestCreateHttpClientWithHttp1AndAuthEnabled() {
	sc := GetDefaultStorageClientConfig(keyFile) // By default http1 enabled
	sc.AnonymousAccess = false

	// Act: this method add tokenSource and clientOptions.
	httpClient, err := CreateHttpClient(&sc, nil)

	assert.NoError(t.T(), err)
	assert.NotNil(t.T(), httpClient)
}

func (t *clientTest) TestCreateHttpClientWithHttp2AndAuthEnabled() {
	sc := GetDefaultStorageClientConfig(keyFile)
	sc.AnonymousAccess = false
	// Act: this method add tokenSource and clientOptions.
	httpClient, err := CreateHttpClient(&sc, nil)

	assert.NoError(t.T(), err)
	assert.NotNil(t.T(), httpClient)
}

func (t *clientTest) TestCreateTokenSrc() {
	sc := GetDefaultStorageClientConfig(keyFile)

	tokenSrc, err := CreateTokenSource(&sc)

	assert.NoError(t.T(), err)
	assert.NotNil(t.T(), &tokenSrc)
}

func (t *clientTest) TestStripScheme() {
	for _, tc := range []struct {
		input          string
		expectedOutput string
	}{
		{
			input:          "",
			expectedOutput: "",
		},
		{
			input:          "localhost:8080",
			expectedOutput: "localhost:8080",
		},
		{
			input:          "http://localhost:8888",
			expectedOutput: "localhost:8888",
		},
		{
			input:          "cp://localhost:8888",
			expectedOutput: "localhost:8888",
		},
		{
			input:          "bad://http://localhost:888://",
			expectedOutput: "http://localhost:888://",
		},
		{
			input:          "dns:///localhost:888://",
			expectedOutput: "dns:///localhost:888://",
		},
		{
			input:          "google-c2p:///localhost:888://",
			expectedOutput: "google-c2p:///localhost:888://",
		},
		{
			input:          "google:///localhost:888://",
			expectedOutput: "google:///localhost:888://",
		},
	} {
		output := StripScheme(tc.input)

		assert.Equal(t.T(), tc.expectedOutput, output)
	}
}
