// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package msalgo

import (
	"reflect"
	"testing"

	"github.com/AzureAD/microsoft-authentication-library-for-go/src/internal/msalbase"
)

func TestAugmentAuthenticationParametersForUsernamePass(t *testing.T) {
	testScopes := []string{"user.read"}
	testUsername := "testUser"
	testPassword := "testPass"
	testAuthParams := &msalbase.AuthParametersInternal{}
	testTokenCommonParams := &acquireTokenCommonParameters{testScopes}
	tokenUserPassParams := &AcquireTokenUsernamePasswordParameters{
		commonParameters: testTokenCommonParams,
		username:         testUsername,
		password:         testPassword,
	}
	tokenUserPassParams.augmentAuthenticationParameters(testAuthParams)
	actualScopes := testAuthParams.Scopes
	if !reflect.DeepEqual(testScopes, actualScopes) {
		t.Errorf("Actual scopes %v differ from expected scopes %v", actualScopes, testScopes)
	}
	actualUsername := testAuthParams.Username
	if !reflect.DeepEqual(testUsername, actualUsername) {
		t.Errorf("Actual username %v differs from expected username %v", actualUsername, testUsername)
	}
	actualPassword := testAuthParams.Password
	if !reflect.DeepEqual(testPassword, actualPassword) {
		t.Errorf("Actual password %v differs from expected password %v", actualPassword, testPassword)
	}
}
