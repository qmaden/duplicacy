// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// The Azure tests run against Azurite, the Azure Storage emulator, when these variables name an instance, for
// example after
//
//	docker run -d -p 127.0.0.1:10000:10000 -e AZURITE_ACCOUNTS="test:$KEY" \
//	    mcr.microsoft.com/azure-storage/azurite azurite-blob --blobHost 0.0.0.0
//
// with DUPLICACY_TEST_AZURITE_URL=http://127.0.0.1:10000/test, DUPLICACY_TEST_AZURITE_ACCOUNT=test and
// DUPLICACY_TEST_AZURITE_KEY=$KEY (any base64 key).

func azuriteSettings() (serviceURL string, account string, key string) {
	return os.Getenv("DUPLICACY_TEST_AZURITE_URL"), os.Getenv("DUPLICACY_TEST_AZURITE_ACCOUNT"),
		os.Getenv("DUPLICACY_TEST_AZURITE_KEY")
}

// createAzuriteContainer creates a container with a random name and returns the name.
func createAzuriteContainer() (string, error) {
	serviceURL, account, key := azuriteSettings()
	random := make([]byte, 6)
	rand.Read(random)
	name := "duplicacy-" + hex.EncodeToString(random)

	credential, err := container.NewSharedKeyCredential(account, key)
	if err != nil {
		return "", err
	}
	client, err := container.NewClientWithSharedKeyCredential(strings.TrimSuffix(serviceURL, "/")+"/"+name, credential, nil)
	if err != nil {
		return "", err
	}
	_, err = client.Create(context.Background(), nil)
	return name, err
}

func TestAzureStorage(t *testing.T) {
	serviceURL, account, key := azuriteSettings()
	if serviceURL == "" {
		t.Skip("DUPLICACY_TEST_AZURITE_URL is not set")
	}
	setTestingT(t)

	if _, err := createAzureStorage(serviceURL, account, key, "missing-container", 1); err == nil ||
		!strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Opening a missing container returned %v", err)
	}

	name, err := createAzuriteContainer()
	if err != nil {
		t.Fatal(err)
	}
	storage, err := createAzureStorage(serviceURL, account, key, name, 4)
	if err != nil {
		t.Fatal(err)
	}
	exerciseCloudStorage(t, storage, 1003)
}
