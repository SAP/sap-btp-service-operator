/*
 * Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package sm

// ClientConfig contains the configuration of the Service Manager client
type ClientConfig struct {
	URL            string
	TokenURL       string
	ClientID       string
	ClientSecret   string
	TokenURLSuffix string
	TLSCertKey     string
	TLSPrivateKey  string
	SSLDisabled    bool
}

func (c ClientConfig) IsValid() bool {
	if len(c.ClientID) == 0 || len(c.URL) == 0 || len(c.TokenURL) == 0 {
		return false
	}
	if len(c.ClientSecret) == 0 && (len(c.TLSCertKey) == 0 || len(c.TLSPrivateKey) == 0) {
		return false
	}

	return true
}
