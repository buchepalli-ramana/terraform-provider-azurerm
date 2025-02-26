package resource

import (
	"context"
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See NOTICE.txt in the project root for license information.

type SpatialAnchorsAccountsCreateOperationResponse struct {
	HttpResponse *http.Response
	Model        *SpatialAnchorsAccount
}

// SpatialAnchorsAccountsCreate ...
func (c ResourceClient) SpatialAnchorsAccountsCreate(ctx context.Context, id SpatialAnchorsAccountId, input SpatialAnchorsAccount) (result SpatialAnchorsAccountsCreateOperationResponse, err error) {
	req, err := c.preparerForSpatialAnchorsAccountsCreate(ctx, id, input)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resource.ResourceClient", "SpatialAnchorsAccountsCreate", nil, "Failure preparing request")
		return
	}

	result.HttpResponse, err = c.Client.Send(req, azure.DoRetryWithRegistration(c.Client))
	if err != nil {
		err = autorest.NewErrorWithError(err, "resource.ResourceClient", "SpatialAnchorsAccountsCreate", result.HttpResponse, "Failure sending request")
		return
	}

	result, err = c.responderForSpatialAnchorsAccountsCreate(result.HttpResponse)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resource.ResourceClient", "SpatialAnchorsAccountsCreate", result.HttpResponse, "Failure responding to request")
		return
	}

	return
}

// preparerForSpatialAnchorsAccountsCreate prepares the SpatialAnchorsAccountsCreate request.
func (c ResourceClient) preparerForSpatialAnchorsAccountsCreate(ctx context.Context, id SpatialAnchorsAccountId, input SpatialAnchorsAccount) (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": defaultApiVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(c.baseUri),
		autorest.WithPath(id.ID()),
		autorest.WithJSON(input),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// responderForSpatialAnchorsAccountsCreate handles the response to the SpatialAnchorsAccountsCreate request. The method always
// closes the http.Response Body.
func (c ResourceClient) responderForSpatialAnchorsAccountsCreate(resp *http.Response) (result SpatialAnchorsAccountsCreateOperationResponse, err error) {
	err = autorest.Respond(
		resp,
		azure.WithErrorUnlessStatusCode(http.StatusCreated, http.StatusOK),
		autorest.ByUnmarshallingJSON(&result.Model),
		autorest.ByClosing())
	result.HttpResponse = resp
	return
}
