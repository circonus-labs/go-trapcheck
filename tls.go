// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func (tc *TrapCheck) clearTLSConfig() {
	tc.broker = nil        // force refresh
	tc.tlsConfig = nil     // don't use, refresh and reset
	tc.custTLSConfig = nil // don't use, refresh and reset
}

// setBrokerTLSConfig sets the broker tls configuration if was
// not supplied by the caller in the configuration.
func (tc *TrapCheck) setBrokerTLSConfig() error {

	// setBrokerTLSConfig has already initialized it
	if tc.tlsConfig != nil {
		return nil
	}

	u, err := url.Parse(tc.submissionURL)
	if err != nil {
		return fmt.Errorf("parse submission URL: %w", err)
	}

	if u.Scheme == "http" {
		return nil // not using tls
	}

	// caller supplied tls config
	if tc.custTLSConfig != nil {
		tc.tlsConfig = tc.custTLSConfig.Clone()
		return nil
	}

	var public bool
	public, err = tc.isPublicBroker()
	if err != nil {
		return err
	}
	if public {
		return nil // public cert
	}

	if tc.broker == nil {
		if tc.checkBundle == nil {
			return fmt.Errorf("invalid state, check bundle not initialized")
		}
		if len(tc.checkBundle.Brokers) == 0 {
			return fmt.Errorf("invalid check bundle, 0 brokers")
		}
		if err = tc.fetchBroker(tc.checkBundle.Brokers[0], tc.checkBundle.Type); err != nil {
			return err
		}
	}

	cn, cnList, err := tc.getBrokerCNList()
	if err != nil {
		return fmt.Errorf("broker cn list: %w", err)
	}

	certPool := x509.NewCertPool()
	cert, err := tc.fetchCert()
	if err != nil {
		return fmt.Errorf("fetch broker ca cert: %w", err)
	}
	if !certPool.AppendCertsFromPEM(cert) {
		return fmt.Errorf("unable to append cert to pool")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: cn,
		// go1.15+ see VerifyConnection below - until CN added to SAN in broker certs
		// NOTE: InsecureSkipVerify:true does NOT disable VerifyConnection()
		InsecureSkipVerify: true, //nolint:gosec
		VerifyConnection: func(cs tls.ConnectionState) error {
			commonName := cs.PeerCertificates[0].Subject.CommonName
			// if commonName != cs.ServerName {
			if !strings.Contains(cnList, commonName) {
				return x509.CertificateInvalidError{
					Cert:   cs.PeerCertificates[0],
					Reason: x509.NameMismatch,
					Detail: fmt.Sprintf("cn: %q, acceptable: %q", commonName, cnList),
				}
			}
			opts := x509.VerifyOptions{
				Roots:         certPool,
				Intermediates: x509.NewCertPool(),
			}
			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			if err != nil {
				return fmt.Errorf("peer cert verify: %w", err)
			}
			return nil
		},
	}

	tc.tlsConfig = tlsConfig

	return nil
}

// caCert contains broker CA certificate returned from Circonus API.
type caCert struct {
	Contents string `json:"contents"`
}

// fetchCert fetches CA certificate using Circonus API.
func (tc *TrapCheck) fetchCert() ([]byte, error) {

	tc.Log.Debugf("fetching broker cert from api")

	response, err := tc.client.Get("/pki/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("fetch broker CA cert from API: %w", err)
	}

	cadata := new(caCert)
	if err := json.Unmarshal(response, cadata); err != nil {
		return nil, fmt.Errorf("json unmarshal cert: %w", err)
	}

	if cadata.Contents == "" {
		return nil, fmt.Errorf("unable to find ca cert contents %+v", cadata)
	}

	return []byte(cadata.Contents), nil
}
