package rhttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
)

var ErrHttpMovedPermanently = errors.New("moved permanently")

type ClientOption func(*retryablehttp.Client)

func WithErrorIfMovedPermanently() ClientOption {
	return func(c *retryablehttp.Client) {
		c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			// default behavior
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}

			if req.Response.StatusCode == http.StatusMovedPermanently {
				return ErrHttpMovedPermanently
			}

			return nil
		}

		c.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
			if resp != nil && resp.StatusCode == http.StatusMovedPermanently {
				return false, err
			}

			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
	}
}

func WithLogger(logger interface{}) ClientOption {
	return func(c *retryablehttp.Client) {
		c.Logger = logger
	}
}

func WithPassthroughErrorHandler() ClientOption {
	return func(c *retryablehttp.Client) {
		c.ErrorHandler = retryablehttp.PassthroughErrorHandler
	}
}

func NewClient(opts ...ClientOption) *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()

	for _, opt := range opts {
		opt(retryClient)
	}

	return retryClient
}
