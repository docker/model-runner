// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

//go:build linux || darwin || aix

package shared

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
)

// FQDNWithContext attempts to lookup the host's fully-qualified domain name and returns it.
// It does so using the following algorithm:
//
//  1. It gets the hostname from the OS. If this step fails, it returns an error.
//
//  2. It tries to perform a CNAME DNS lookup for the hostname. If this succeeds, it
//     returns the CNAME (after trimming any trailing period) as the FQDN.
//
//  3. It tries to perform an IP lookup for the hostname. If this succeeds, it tries
//     to perform a reverse DNS lookup on the returned IPs and returns the first
//     successful result (after trimming any trailing period) as the FQDN.
//
//  4. If steps 2 and 3 both fail, an empty string is returned as the FQDN along with
//     errors from those steps.
func FQDNWithContext(ctx context.Context) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("could not get hostname to look for FQDN: %w", err)
	}

	return fqdn(ctx, hostname)
}

// FQDN just calls FQDNWithContext with a background context.
// Deprecated.
func FQDN() (string, error) {
	return FQDNWithContext(context.Background())
}

func fqdn(ctx context.Context, hostname string) (string, error) {
	var errs error
	cname, err := net.DefaultResolver.LookupCNAME(ctx, hostname)
	if err != nil {
		errs = fmt.Errorf("could not get FQDN, all methods failed: failed looking up CNAME: %w",
			err)
	}

	if cname != "" {
		cname = strings.TrimSuffix(cname, ".")

		// Go might lowercase the cname "for convenience". Therefore, if cname
		// is the same as hostname, return hostname as is.
		// See https://github.com/golang/go/blob/go1.22.5/src/net/hosts.go#L38
		if strings.ToLower(cname) == strings.ToLower(hostname) {
			return hostname, nil
		}

		return cname, nil
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", hostname)
	if err != nil {
		errs = fmt.Errorf("%s: failed looking up IP: %w", errs, err)
	}

	for _, ip := range ips {
		names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
		if err != nil || len(names) == 0 {
			continue
		}
		return strings.TrimSuffix(names[0], "."), nil
	}

	return "", errs
}
