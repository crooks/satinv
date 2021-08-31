// satapi provides an API interface to Red Hat Satellite
package satapi

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

// AuthClient contains the HTTP client components
type AuthClient struct {
	Username   string
	Password   string
	HTTPClient *http.Client
}

// NewBasicAuthClient returns an instance of AuthClient
func NewBasicAuthClient(username, password, certFile string) *AuthClient {
	return &AuthClient{
		Username:   username,
		Password:   password,
		HTTPClient: httpAuthClient(certFile),
	}
}

// GetJSON takes a URL relating to a Rest API and returns the resulting JSON as a byte slice.
func (s *AuthClient) GetJSON(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	bytes, err := s.doRequest(req)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

// httpAuthClient creates a new instance of http.Client with support for
// additional rootCAs.  As XClarity is frequently installed as an appliance,
// with a self-signed cert, this appears to be quite useful.
func httpAuthClient(certFile string) *http.Client {
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal(err)
	}
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	certs, err := ioutil.ReadFile(certFile)
	if errors.Is(err, os.ErrNotExist) {
		//log.Println("No additional certificates imported")
	} else if err != nil {
		panic(err)
	} else if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		log.Println("Cert import failed.  Proceeding with system CAs.")
	}
	config := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            rootCAs,
	}
	tr := &http.Transport{TLSClientConfig: config}
	return &http.Client{Transport: tr}
}

// doRequest does an HTTP URL request and returns it as a byte array
func (s *AuthClient) doRequest(req *http.Request) ([]byte, error) {
	req.SetBasicAuth(s.Username, s.Password)
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status error: %s\n", string(body))
	}
	return body, nil
}
