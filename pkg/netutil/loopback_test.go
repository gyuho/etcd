// Copyright 2018 The etcd Authors
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

package netutil

import "testing"

func TestGetHostIsLoopback(t *testing.T) {
	tt := []struct {
		addr       string
		host       string
		isLoopback bool
	}{
		{"localhost", "localhost", true},
		{"localhost:2379", "localhost", true},
		{"localhost.", "localhost.", true},
		{"localhost.:2379", "localhost.", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1:2379", "127.0.0.1", true},

		{"http://localhost", "localhost", true},
		{"http://localhost:2379", "localhost", true},
		{"http://localhost.", "localhost.", true},
		{"http://localhost.:2379", "localhost.", true},
		{"http://127.0.0.1", "127.0.0.1", true},
		{"http://127.0.0.1:2379", "127.0.0.1", true},

		{"https://localhost", "localhost", true},
		{"https://localhost:2379", "localhost", true},
		{"https://localhost.", "localhost.", true},
		{"https://localhost.:2379", "localhost.", true},
		{"https://127.0.0.1", "127.0.0.1", true},
		{"https://127.0.0.1:2379", "127.0.0.1", true},

		{"localhos", "localhos", false},
		{"localhos:2379", "localhos", false},
		{"localhos.", "localhos.", false},
		{"localhos.:2379", "localhos.", false},
		{"1.2.3.4", "1.2.3.4", false},
		{"1.2.3.4:2379", "1.2.3.4", false},

		{"http://localhos", "localhos", false},
		{"http://localhos:2379", "localhos", false},
		{"http://localhos.", "localhos.", false},
		{"http://localhos.:2379", "localhos.", false},
		{"http://1.2.3.4", "1.2.3.4", false},
		{"http://1.2.3.4:2379", "1.2.3.4", false},

		{"localhost:::::", "localhost:::::", false},
	}
	for i := range tt {
		hv := GetHost(tt[i].addr)
		if hv != tt[i].host {
			t.Errorf("#%d: %q expected host %q, got '%v'", i, tt[i].addr, tt[i].host, hv)
		}
		lv := IsLoopback(tt[i].addr)
		if lv != tt[i].isLoopback {
			t.Errorf("#%d: %q expected loopback '%v', got '%v'", i,
				tt[i].addr, tt[i].isLoopback, lv)
		}
	}
}
