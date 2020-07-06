// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package tokencache

import (
	"errors"
	"strconv"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/src/internal/msalbase"
	"github.com/AzureAD/microsoft-authentication-library-for-go/src/internal/requests"

	log "github.com/sirupsen/logrus"
)

type cacheManager struct {
	storageManager    IStorageManager
	cacheAccessAspect ICacheAccessAspect
}

func CreateCacheManager(storageManager IStorageManager) *cacheManager {
	cache := &cacheManager{storageManager: storageManager}
	return cache
}

func isAccessTokenValid(accessToken *accessTokenCacheItem) bool {
	cachedAt, err := strconv.ParseInt(accessToken.CachedAt, 10, 64)
	if err != nil {
		log.Info("This access token isn't valid, it was cached at an invalid time.")
		return false
	}
	now := time.Now().Unix()
	if cachedAt > now {
		log.Info("This access token isn't valid, it was cached at an invalid time.")
		return false
	}
	expiresOn, err := strconv.ParseInt(accessToken.ExpiresOnUnixTimestamp, 10, 64)
	if err != nil {
		log.Info("This access token isn't valid, it expires at an invalid time.")
		return false
	}
	if expiresOn <= now+300 {
		log.Info("This access token is expired")
		return false
	}
	return true
}

func (m *cacheManager) GetAllAccounts() []*msalbase.Account {
	return m.storageManager.ReadAllAccounts()
}

func (m *cacheManager) TryReadCache(authParameters *msalbase.AuthParametersInternal, webRequestManager requests.IWebRequestManager) (*msalbase.StorageTokenResponse, error) {
	homeAccountID := authParameters.HomeaccountID
	realm := authParameters.AuthorityInfo.UserRealmURIPrefix
	clientID := authParameters.ClientID
	scopes := authParameters.Scopes
	aadInstanceDiscovery := requests.CreateAadInstanceDiscovery(webRequestManager)
	metadata, err := aadInstanceDiscovery.GetMetadataEntry(authParameters.AuthorityInfo)
	if err != nil {
		return nil, err
	}
	log.Tracef("Querying the cache for homeAccountId '%s' environments '%v' realm '%s' clientId '%s' scopes:'%v'", homeAccountID, metadata.Aliases, realm, clientID, scopes)
	if homeAccountID == "" || len(metadata.Aliases) == 0 || realm == "" || clientID == "" || len(scopes) == 0 {
		log.Warn("Skipping the tokens cache lookup, one of the primary keys is empty")
		return nil, errors.New("Skipping the tokens cache lookup, one of the primary keys is empty")
	}
	accessToken := m.storageManager.ReadAccessToken(homeAccountID, metadata.Aliases, realm, clientID, scopes)
	if accessToken != nil {
		if !isAccessTokenValid(accessToken) {
			accessToken = nil
		}
	}
	idToken := m.storageManager.ReadIDToken(homeAccountID, metadata.Aliases, realm, clientID)
	var familyID string
	appMetadata := m.storageManager.ReadAppMetadata(metadata.Aliases, clientID)
	if appMetadata == nil {
		familyID = ""
	} else {
		familyID = appMetadata.FamilyID
	}
	refreshToken := m.storageManager.ReadRefreshToken(homeAccountID, metadata.Aliases, familyID, clientID)
	account := m.storageManager.ReadAccount(homeAccountID, metadata.Aliases, realm)
	return msalbase.CreateStorageTokenResponse(accessToken, refreshToken, idToken, account), nil
}

func (m *cacheManager) CacheTokenResponse(authParameters *msalbase.AuthParametersInternal, tokenResponse *msalbase.TokenResponse) (*msalbase.Account, error) {
	var err error
	log.Infof("%v", authParameters.AuthorityInfo)
	authParameters.HomeaccountID = tokenResponse.GetHomeAccountIDFromClientInfo()
	homeAccountID := authParameters.HomeaccountID
	environment := authParameters.AuthorityInfo.Host
	realm := authParameters.AuthorityInfo.UserRealmURIPrefix
	clientID := authParameters.ClientID
	target := msalbase.ConcatenateScopes(tokenResponse.GrantedScopes)

	log.Infof("Writing to the cache for homeAccountId '%s' environment '%s' realm '%s' clientId '%s' target '%s'", homeAccountID, environment, realm, clientID, target)

	if homeAccountID == "" || environment == "" || realm == "" || clientID == "" || target == "" {
		return nil, errors.New("Skipping writing data to the tokens cache, one of the primary keys is empty")
	}

	cachedAt := time.Now().Unix()

	if tokenResponse.HasRefreshToken() {
		refreshToken := CreateRefreshTokenCacheItem(homeAccountID, environment, clientID, tokenResponse.RefreshToken, tokenResponse.FamilyID)
		err = m.storageManager.WriteRefreshToken(refreshToken)
		if err != nil {
			return nil, err
		}
	}

	if tokenResponse.HasAccessToken() {
		expiresOn := tokenResponse.ExpiresOn.Unix()
		extendedExpiresOn := tokenResponse.ExtExpiresOn.Unix()
		accessToken := CreateAccessTokenCacheItem(homeAccountID,
			environment,
			realm,
			clientID,
			cachedAt,
			expiresOn,
			extendedExpiresOn,
			target,
			tokenResponse.AccessToken)
		if isAccessTokenValid(accessToken) {
			err = m.storageManager.WriteAccessToken(accessToken)
			if err != nil {
				return nil, err
			}
		}
	}

	idTokenJwt := tokenResponse.IDToken

	idToken := CreateIDTokenCacheItem(homeAccountID, environment, realm, clientID, idTokenJwt.RawToken)
	m.storageManager.WriteIDToken(idToken)

	if err != nil {
		return nil, err
	}

	localAccountID := idTokenJwt.GetLocalAccountID()
	authorityType := authParameters.AuthorityInfo.AuthorityType

	account := msalbase.CreateAccount(
		homeAccountID,
		environment,
		realm,
		localAccountID,
		authorityType,
		idTokenJwt.PreferredUsername,
	)

	err = m.storageManager.WriteAccount(account)

	if err != nil {
		return nil, err
	}

	appMetadata := CreateAppMetadata(tokenResponse.FamilyID, clientID, environment)

	err = m.storageManager.WriteAppMetadata(appMetadata)

	if err != nil {
		return nil, err
	}

	return account, nil
}

func (m *cacheManager) DeleteCachedRefreshToken(authParameters *msalbase.AuthParametersInternal) error {
	homeAccountID := "" // todo: authParameters.GetAccountId()
	environment := ""   // authParameters.GetAuthorityInfo().GetEnvironment()
	clientID := authParameters.ClientID

	emptyCorrelationID := ""
	emptyRealm := ""
	emptyFamilyID := ""
	emptyTarget := ""

	log.Infof("Deleting refresh token from the cache for homeAccountId '%s' environment '%s' clientID '%s'", homeAccountID, environment, clientID)

	if homeAccountID == "" || environment == "" || clientID == "" {
		log.Warn("Failed to delete refresh token from the cache, one of the primary keys is empty")
		return errors.New("Failed to delete refresh token from the cache, one of the primary keys is empty")
	}

	operationStatus, err := m.storageManager.DeleteCredentials(emptyCorrelationID, homeAccountID, environment, emptyRealm, clientID, emptyFamilyID, emptyTarget, map[msalbase.CredentialType]bool{msalbase.CredentialTypeOauth2RefreshToken: true})
	if err != nil {
		return nil
	}

	if operationStatus.StatusType != OperationStatusTypeSuccess {
		log.Warn("Error deleting an invalid refresh token from the cache")
	}

	return nil
}

func (m *cacheManager) deleteCachedAccessToken(homeAccountID string, environment string, realm string, clientID string, target string) error {
	log.Infof("Deleting an access token from the cache for homeAccountId '%s' environment '%s' realm '%s' clientId '%s' target '%s'", homeAccountID, environment, realm, clientID, target)

	emptyCorrelationID := ""
	emptyFamilyID := ""

	operationStatus, err := m.storageManager.DeleteCredentials(emptyCorrelationID, homeAccountID, environment, realm, clientID, emptyFamilyID, target, map[msalbase.CredentialType]bool{msalbase.CredentialTypeOauth2AccessToken: true})

	if err != nil {
		return err
	}

	if operationStatus.StatusType != OperationStatusTypeSuccess {
		log.Warn("Failure deleting an access token from the cache")
	}
	return nil
}
