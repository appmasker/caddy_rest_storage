package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/certmagic"
	"go.uber.org/zap"
)

type RestStorage struct {
	Endpoint string `json:"endpoint"`
	ApiKey   string `json:"api_key"`
	logger 	 *zap.Logger 
}

func init() {
	caddy.RegisterModule(new(RestStorage))
}

func (r RestStorage) client(ctx context.Context, method string, path string, dataStruct any) (*http.Response, error) {
	httpClient := &http.Client{}
	requestBody, err := json.Marshal(dataStruct)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, r.Endpoint+path, bytes.NewBuffer(requestBody))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("x-api-key", r.ApiKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (RestStorage) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.storage.rest",
		New: func() caddy.Module { return new(RestStorage) },
	}
}

func (r *RestStorage) Provision(ctx caddy.Context) error {
	if !strings.HasSuffix(r.Endpoint, "/") {
		r.Endpoint = r.Endpoint + "/"
	}

	repl := caddy.NewReplacer()
	r.ApiKey = repl.ReplaceAll(r.ApiKey, "")
	r.logger = ctx.Logger(r)
	return nil
}

func (r RestStorage) Validate() error {
	if r.Endpoint == "" {
		return errors.New("endpoint must be specified")
	}

	if r.ApiKey == "" {
		return errors.New("api key must be defined")
	}

	return nil
}

func (r *RestStorage) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		var value string

		key := d.Val()

		if !d.Args(&value) {
			continue
		}

		switch key {
		case "endpoint":
			r.Endpoint = value
		case "apikey":
		case "apiKey":
		case "ApiKey":
			r.ApiKey = value
		}
	}

	return nil
}

func (r *RestStorage) CertMagicStorage() (certmagic.Storage, error) {
	return r, nil
}

type LockRequest struct {
	Key string `json:"key"`
}

func (r *RestStorage) Lock(ctx context.Context, key string) error {
	for {
		resp, err := r.client(ctx, "POST", "lock", LockRequest{Key: key})

		if err != nil {
			return err
		}

		resp.Body.Close()

		// The key was successfully locked
		if resp.StatusCode == 201 {
			return nil
		}

		if resp.StatusCode == 423 {
			// 423: The key is already locked
			r.logger.Info(fmt.Sprintf("Key %v is already locked.", key))
		} else if resp.StatusCode == 412 {
			// 412: An error occurred
			r.logger.Error(fmt.Sprintf("Error locking key %v: %v ; Will try again.\n", key, resp.StatusCode))
		} else {
			// unknown error. return it
			return fmt.Errorf("Unknown status code received: %v", resp.StatusCode)
		}

		// Wait 5 seconds before trying again
		time.Sleep(5 * time.Second)
	}
}

type UnlockRequest struct {
	Key string `json:"key"`
}

func (r *RestStorage) Unlock(ctx context.Context, key string) error {
	resp, err := r.client(ctx, "POST", "unlock", UnlockRequest{
		Key: key,
	})

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	return nil
}

type StoreRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (r *RestStorage) Store(ctx context.Context, key string, value []byte) error {
	valueEnc := base64.StdEncoding.EncodeToString(value)
	resp, err := r.client(ctx, "POST", "store", StoreRequest{
		Key:   key,
		Value: valueEnc,
	})

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		return fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	return nil
}

type LoadRequest struct {
	Key string `json:"key"`
}

type LoadResponse struct {
	Value string `json:"value"`
}

func (r *RestStorage) Load(ctx context.Context, key string) ([]byte, error) {
	resp, err := r.client(ctx, "POST", "load", LoadRequest{
		Key: key,
	})

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fs.ErrNotExist
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	var loadResp LoadResponse

	err = json.NewDecoder(resp.Body).Decode(&loadResp)

	if err != nil {
		return nil, err
	}

	valueDec, err := base64.StdEncoding.DecodeString(loadResp.Value)

	if err != nil {
		return nil, err
	}

	return valueDec, nil
}

type DeleteRequest struct {
	Key string `json:"key"`
}

func (r *RestStorage) Delete(ctx context.Context, key string) error {
	resp, err := r.client(ctx, "DELETE", "delete", DeleteRequest{
		Key: key,
	})

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fs.ErrNotExist
	}

	if resp.StatusCode != 204 {
		return fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	return nil
}

type ExistsRequest struct {
	Key string `json:"key"`
}

type ExistsResponse struct {
	Exists bool `json:"exists"`
}

func (r *RestStorage) Exists(ctx context.Context, key string) bool {
	resp, err := r.client(ctx, "POST", "exists", ExistsRequest{
		Key: key,
	})

	if err != nil {
		return false
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	var existsResp ExistsResponse

	err = json.NewDecoder(resp.Body).Decode(&existsResp)

	if err != nil {
		return false
	}

	return existsResp.Exists
}

type ListRequest struct {
	Prefix    string `json:"prefix"`
	Recursive bool   `json:"recursive"`
}

type ListResponse struct {
	Keys []string `json:"keys"`
}

func (r *RestStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	resp, err := r.client(ctx, "POST", "list", ListRequest{
		Prefix:    prefix,
		Recursive: recursive,
	})

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fs.ErrNotExist
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	var listResp ListResponse

	err = json.NewDecoder(resp.Body).Decode(&listResp)

	if err != nil {
		return nil, err
	}

	return listResp.Keys, nil
}

type StatRequest struct {
	Key string `json:"key"`
}

type StatResponse struct {
	Key        string `json:"key"`
	Modified   string `json:"modified"`
	Size       int64  `json:"size"`
	IsTerminal bool   `json:"isTerminal"`
}

func (r *RestStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	resp, err := r.client(ctx, "POST", "stat", StatRequest{
		Key: key,
	})

	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return certmagic.KeyInfo{}, fs.ErrNotExist
	}

	if resp.StatusCode != 200 {
		return certmagic.KeyInfo{}, fmt.Errorf("unknown status code received: %v", resp.StatusCode)
	}

	var statResp StatResponse

	err = json.NewDecoder(resp.Body).Decode(&statResp)

	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	parsedTime, err := time.Parse(time.RFC3339, statResp.Modified)

	if err != nil {
		return certmagic.KeyInfo{}, err
	}

	return certmagic.KeyInfo{
		Key:        statResp.Key,
		Modified:   parsedTime,
		Size:       statResp.Size,
		IsTerminal: statResp.IsTerminal,
	}, nil
}
