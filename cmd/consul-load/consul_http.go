package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-cleanhttp"
)

// StringValue provides a flag value that's aware if it has been set.
type StringValue struct {
	v *string
}

// Merge will overlay this value if it has been set.
func (s *StringValue) Merge(onto *string) {
	if s.v != nil {
		*onto = *(s.v)
	}
}

// Set implements the flag.Value interface.
func (s *StringValue) Set(v string) error {
	if s.v == nil {
		s.v = new(string)
	}
	*(s.v) = v
	return nil
}

// String implements the flag.Value interface.
func (s *StringValue) String() string {
	var current string
	if s.v != nil {
		current = *(s.v)
	}
	return current
}

// BoolValue provides a flag value that's aware if it has been set.
type BoolValue struct {
	v *bool
}

// IsBoolFlag is an optional method of the flag.Value
// interface which marks this value as boolean when
// the return value is true. See flag.Value for details.
func (b *BoolValue) IsBoolFlag() bool {
	return true
}

// Merge will overlay this value if it has been set.
func (b *BoolValue) Merge(onto *bool) {
	if b.v != nil {
		*onto = *(b.v)
	}
}

// Set implements the flag.Value interface.
func (b *BoolValue) Set(v string) error {
	if b.v == nil {
		b.v = new(bool)
	}
	var err error
	*(b.v), err = strconv.ParseBool(v)
	return err
}

// String implements the flag.Value interface.
func (b *BoolValue) String() string {
	var current bool
	if b.v != nil {
		current = *(b.v)
	}
	return fmt.Sprintf("%v", current)
}

type HTTPFlags struct {
	// client api flags
	address       StringValue
	token         StringValue
	tokenFile     StringValue
	caFile        StringValue
	caPath        StringValue
	certFile      StringValue
	keyFile       StringValue
	tlsServerName StringValue

	// server flags
	datacenter StringValue
	stale      BoolValue

	// namespace flags
	namespace StringValue
}

func (f *HTTPFlags) MergeAll(main *flag.FlagSet) {
	FlagMerge(main, f.ClientFlags())
	FlagMerge(main, f.ServerFlags())
	FlagMerge(main, f.NamespaceFlags())
}

func (f *HTTPFlags) ClientFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.address, "http-addr",
		"The `address` and port of the Consul HTTP agent. The value can be an IP "+
			"address or DNS address, but it must also include the port. This can "+
			"also be specified via the CONSUL_HTTP_ADDR environment variable. The "+
			"default value is http://127.0.0.1:8500. The scheme can also be set to "+
			"HTTPS by setting the environment variable CONSUL_HTTP_SSL=true.")
	fs.Var(&f.token, "token",
		"ACL token to use in the request. This can also be specified via the "+
			"CONSUL_HTTP_TOKEN environment variable. If unspecified, the query will "+
			"default to the token of the Consul agent at the HTTP address.")
	fs.Var(&f.tokenFile, "token-file",
		"File containing the ACL token to use in the request instead of one specified "+
			"via the -token argument or CONSUL_HTTP_TOKEN environment variable. "+
			"This can also be specified via the CONSUL_HTTP_TOKEN_FILE environment variable.")
	fs.Var(&f.caFile, "ca-file",
		"Path to a CA file to use for TLS when communicating with Consul. This "+
			"can also be specified via the CONSUL_CACERT environment variable.")
	fs.Var(&f.caPath, "ca-path",
		"Path to a directory of CA certificates to use for TLS when communicating "+
			"with Consul. This can also be specified via the CONSUL_CAPATH environment variable.")
	fs.Var(&f.certFile, "client-cert",
		"Path to a client cert file to use for TLS when 'verify_incoming' is enabled. This "+
			"can also be specified via the CONSUL_CLIENT_CERT environment variable.")
	fs.Var(&f.keyFile, "client-key",
		"Path to a client key file to use for TLS when 'verify_incoming' is enabled. This "+
			"can also be specified via the CONSUL_CLIENT_KEY environment variable.")
	fs.Var(&f.tlsServerName, "tls-server-name",
		"The server name to use as the SNI host when connecting via TLS. This "+
			"can also be specified via the CONSUL_TLS_SERVER_NAME environment variable.")
	return fs
}

func (f *HTTPFlags) ServerFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.datacenter, "datacenter",
		"Name of the datacenter to query. If unspecified, this will default to "+
			"the datacenter of the queried agent.")
	fs.Var(&f.stale, "stale",
		"Permit any Consul server (non-leader) to respond to this request. This "+
			"allows for lower latency and higher throughput, but can result in "+
			"stale data. This option has no effect on non-read operations. The "+
			"default value is false.")
	return fs
}

func (f *HTTPFlags) NamespaceFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.namespace, "namespace",
		"Specifies the namespace to query. If not provided, the namespace will be inferred "+
			"from the request's ACL token, or will default to the `default` namespace. "+
			"Namespaces are a Consul Enterprise feature.")
	return fs
}

func (f *HTTPFlags) Addr() string {
	return f.address.String()
}

func (f *HTTPFlags) Datacenter() string {
	return f.datacenter.String()
}

func (f *HTTPFlags) Stale() bool {
	if f.stale.v == nil {
		return false
	}
	return *f.stale.v
}

func (f *HTTPFlags) Token() string {
	return f.token.String()
}

func (f *HTTPFlags) SetToken(v string) error {
	return f.token.Set(v)
}

func (f *HTTPFlags) TokenFile() string {
	return f.tokenFile.String()
}

func (f *HTTPFlags) SetTokenFile(v string) error {
	return f.tokenFile.Set(v)
}

func (f *HTTPFlags) ReadTokenFile() (string, error) {
	tokenFile := f.tokenFile.String()
	if tokenFile == "" {
		return "", nil
	}

	data, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func (f *HTTPFlags) APIClient() (*api.Client, error) {
	c := api.DefaultConfig()

	f.MergeOntoConfig(c)

	trans := cleanhttp.DefaultPooledTransport()
	trans.MaxConnsPerHost = 100
	trans.ForceAttemptHTTP2 = true

	tlsConf, err := api.SetupTLSConfig(&c.TLSConfig)
	if err != nil {
		return nil, err
	}

	if logPath := os.Getenv("CONSUL_TLS_KEY_LOG_FILE"); logPath != "" {
		kl, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return nil, fmt.Errorf("couldn't open the TLS key log file %q for writing: %w", logPath, err)
		}
		tlsConf.KeyLogWriter = kl
	}

	trans.TLSClientConfig = tlsConf
	c.HttpClient = &http.Client{Transport: trans}

	return api.NewClient(c)
}

func (f *HTTPFlags) MergeOntoConfig(c *api.Config) {
	f.address.Merge(&c.Address)
	f.token.Merge(&c.Token)
	f.tokenFile.Merge(&c.TokenFile)
	f.caFile.Merge(&c.TLSConfig.CAFile)
	f.caPath.Merge(&c.TLSConfig.CAPath)
	f.certFile.Merge(&c.TLSConfig.CertFile)
	f.keyFile.Merge(&c.TLSConfig.KeyFile)
	f.tlsServerName.Merge(&c.TLSConfig.Address)
	f.datacenter.Merge(&c.Datacenter)
	f.namespace.Merge(&c.Namespace)
}

func FlagMerge(dst, src *flag.FlagSet) {
	if dst == nil {
		panic("dst cannot be nil")
	}
	if src == nil {
		return
	}
	src.VisitAll(func(f *flag.Flag) {
		dst.Var(f.Value, f.Name, f.Usage)
	})
}
