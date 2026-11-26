/*
 * Copyright (c) 2025, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package utils

import (
	"encoding/json"
	"log"
)

func GetStringSlice(arg map[string]interface{}, key string) []string {
	if val, ok := arg[key]; ok && val != nil {
		if slice, ok := val.([]string); ok {
			return slice
		}
		// fallback: it might be []interface{} with string values
		if ifaceSlice, ok := val.([]interface{}); ok {
			strSlice := make([]string, 0, len(ifaceSlice))
			for _, v := range ifaceSlice {
				if str, ok := v.(string); ok {
					strSlice = append(strSlice, str)
				}
			}
			return strSlice
		}
	}
	return []string{}
}

// GetBoolWithDefault returns the boolean value from the interface{} if it exists,
// otherwise returns the default value
func GetBoolWithDefault(value interface{}, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}

	if boolValue, ok := value.(bool); ok {
		return boolValue
	}

	return defaultValue
}

func GetOptionalParam[T any](args map[string]interface{}, key string) *T {
	if val, ok := args[key]; ok {
		if typedVal, ok := val.(T); ok {
			return &typedVal
		}
	}
	return nil
}

func MarshalResponse(response interface{}) (string, error) {
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return "", err
	}
	return string(jsonData), nil
}
