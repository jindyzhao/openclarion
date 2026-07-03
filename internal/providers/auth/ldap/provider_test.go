package ldap

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"

	gldap "github.com/go-ldap/ldap/v3"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderAuthenticateAuthorizationSearchesAndBindsUser(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,ou=people,dc=example,dc=com", map[string][]string{
					"mail":     {"alice@example.com"},
					"memberOf": {"cn=OPS,ou=groups,dc=example,dc=com"},
				}),
			},
		},
	}
	provider, err := NewProvider(Config{
		URL:              "ldaps://ldap.example.com:636",
		BaseDN:           "dc=example,dc=com",
		BindDN:           "cn=openclarion,ou=svc,dc=example,dc=com",
		BindPassword:     "service-password",
		UserFilter:       "(&(objectClass=person)(uid={username}))",
		SubjectAttribute: "mail",
		OwnerRoleValues:  []string{"cn=ops,ou=groups,dc=example,dc=com"},
		Dialer:           &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	principal, err := provider.AuthenticateAuthorization(context.Background(), basicHeader("alice"))
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}

	if principal.Subject != "alice@example.com" {
		t.Fatalf("subject = %q, want alice@example.com", principal.Subject)
	}
	if !slices.Equal(principal.Roles, []ports.AuthRole{ports.AuthRoleOwner}) {
		t.Fatalf("roles = %#v, want owner", principal.Roles)
	}
	if string(principal.Claims) == "" ||
		strings.Contains(string(principal.Claims), "alice-password") ||
		strings.Contains(string(principal.Claims), "service-password") ||
		strings.Contains(string(principal.Claims), "uid=alice") {
		t.Fatalf("claims leaked credential or dn: %s", string(principal.Claims))
	}
	if !slices.Equal(conn.binds, []bindCall{
		{username: "cn=openclarion,ou=svc,dc=example,dc=com", password: "service-password"},
		{username: "uid=alice,ou=people,dc=example,dc=com", password: "alice-password"},
	}) {
		t.Fatalf("binds = %#v", conn.binds)
	}
	if len(conn.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(conn.searches))
	}
	search := conn.searches[0]
	if search.BaseDN != "dc=example,dc=com" || search.Filter != "(&(objectClass=person)(uid=alice))" {
		t.Fatalf("search = base %q filter %q", search.BaseDN, search.Filter)
	}
	if !slices.Contains(search.Attributes, "mail") || !slices.Contains(search.Attributes, "memberOf") {
		t.Fatalf("search attributes = %#v", search.Attributes)
	}
	if !search.EnforceSizeLimit {
		t.Fatal("search.EnforceSizeLimit = false, want true")
	}
	if !conn.closed {
		t.Fatal("connection was not closed")
	}
}

func TestProviderAuthenticateAuthorizationPreservesServiceBindPassword(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,dc=example,dc=com", nil),
			},
		},
	}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		BindDN:       "cn=openclarion,dc=example,dc=com",
		BindPassword: " service-password ",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if _, err := provider.AuthenticateAuthorization(context.Background(), basicHeader("alice")); err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}

	if len(conn.binds) != 2 {
		t.Fatalf("binds = %#v", conn.binds)
	}
	if conn.binds[0].password != " service-password " {
		t.Fatalf("service bind password = %q, want preserved whitespace", conn.binds[0].password)
	}
}

func TestProviderAuthenticateAuthorizationEscapesUsernameInSearchFilter(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,dc=example,dc=com", nil),
			},
		},
	}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com:636/path",
		BaseDN:       "dc=example,dc=com",
		UserFilter:   "(uid={username})",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	username := "alice*)(uid=*)"
	principal, err := provider.AuthenticateAuthorization(context.Background(), basicHeader(username))
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}

	if principal.Subject != username {
		t.Fatalf("subject = %q, want %q", principal.Subject, username)
	}
	if len(conn.searches) != 1 {
		t.Fatalf("searches = %d, want 1", len(conn.searches))
	}
	wantFilter := "(uid=" + gldap.EscapeFilter(username) + ")"
	if conn.searches[0].Filter != wantFilter {
		t.Fatalf("filter = %q, want %q", conn.searches[0].Filter, wantFilter)
	}
}

func TestProviderSearchAttributesDeduplicatesCaseInsensitiveLDAPAttributes(t *testing.T) {
	provider := &Provider{
		subjectAttribute: "memberOf",
		roleAttribute:    "memberof",
	}

	if got := provider.searchAttributes(); !slices.Equal(got, []string{"memberOf"}) {
		t.Fatalf("searchAttributes = %#v, want only first case-insensitive LDAP attribute", got)
	}
}

func TestProviderReportsNonSensitiveTransportPolicyStatus(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want ports.AuthTransportSecurity
	}{
		{
			name: "ldaps",
			cfg: Config{
				URL:          "ldaps://ldap.example.com:636",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
				Dialer:       &fakeDialer{conn: &fakeConn{}},
			},
			want: ports.AuthTransportSecurityTLS,
		},
		{
			name: "start tls",
			cfg: Config{
				URL:          "ldap://ldap.example.com:389",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
				StartTLS:     true,
				Dialer:       &fakeDialer{conn: &fakeConn{}},
			},
			want: ports.AuthTransportSecurityStartTLS,
		},
		{
			name: "insecure plaintext",
			cfg: Config{
				URL:                    "ldap://ldap.example.com:389",
				BaseDN:                 "dc=example,dc=com",
				DefaultRoles:           []ports.AuthRole{ports.AuthRoleOwner},
				AllowInsecurePlaintext: true,
				Dialer:                 &fakeDialer{conn: &fakeConn{}},
			},
			want: ports.AuthTransportSecurityInsecurePlaintext,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewProvider(tc.cfg)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}

			status := provider.TransportPolicyStatus()
			if status.Security != tc.want {
				t.Fatalf("transport security = %q, want %q", status.Security, tc.want)
			}
		})
	}
}

func TestProviderAuthenticateAuthorizationStartsTLSBeforeBinding(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,dc=example,dc=com", nil),
			},
		},
	}
	provider, err := NewProvider(Config{
		URL:          "ldap://ldap.example.com:389",
		BaseDN:       "dc=example,dc=com",
		BindDN:       "cn=openclarion,dc=example,dc=com",
		BindPassword: "service-password",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		StartTLS:     true,
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), basicHeader("alice"))
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}

	if conn.startTLSCalls != 1 {
		t.Fatalf("start tls calls = %d, want 1", conn.startTLSCalls)
	}
	if conn.startTLSConfig == nil ||
		conn.startTLSConfig.ServerName != "ldap.example.com" ||
		conn.startTLSConfig.MinVersion == 0 {
		t.Fatalf("start tls config = %+v", conn.startTLSConfig)
	}
	if !slices.Equal(conn.operations, []string{
		"start_tls",
		"bind:cn=openclarion,dc=example,dc=com",
		"search",
		"bind:uid=alice,dc=example,dc=com",
	}) {
		t.Fatalf("operations = %#v", conn.operations)
	}
}

func TestProviderAuthenticateAuthorizationRejectsInvalidCredentialsWithoutDialing(t *testing.T) {
	dialer := &fakeDialer{conn: &fakeConn{}}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       dialer,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), "Bearer token")
	if err == nil {
		t.Fatal("AuthenticateAuthorization error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Basic") {
		t.Fatalf("error = %q, want Basic credential error", err)
	}
	if len(dialer.urls) != 0 {
		t.Fatalf("dialed urls = %#v, want none", dialer.urls)
	}
}

func TestProviderAuthenticateAuthorizationRejectsMalformedUsernamesWithoutDialing(t *testing.T) {
	dialer := &fakeDialer{conn: &fakeConn{}}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       dialer,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	tests := []struct {
		name     string
		username string
	}{
		{name: "leading whitespace", username: " alice"},
		{name: "trailing whitespace", username: "alice "},
		{name: "embedded whitespace", username: "ali ce"},
		{name: "control character", username: "alice\x01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := provider.AuthenticateAuthorization(context.Background(), basicHeader(tc.username))
			if err == nil {
				t.Fatal("AuthenticateAuthorization error = nil, want error")
			}
			if !strings.Contains(err.Error(), "invalid basic credentials") {
				t.Fatalf("error = %q, want sanitized invalid basic credentials", err.Error())
			}
		})
	}
	if len(dialer.urls) != 0 {
		t.Fatalf("dialed urls = %#v, want none", dialer.urls)
	}
}

func TestProviderAuthenticateAuthorizationRejectsAmbiguousSearch(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,dc=example,dc=com", nil),
				gldap.NewEntry("uid=alice2,dc=example,dc=com", nil),
			},
		},
	}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), basicHeader("alice"))
	if err == nil {
		t.Fatal("AuthenticateAuthorization error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid credentials") ||
		strings.Contains(err.Error(), "alice-password") {
		t.Fatalf("error = %q, want sanitized invalid credentials", err)
	}
}

func TestProviderAuthenticateAuthorizationSanitizesBindFailure(t *testing.T) {
	conn := &fakeConn{
		searchResult: &gldap.SearchResult{
			Entries: []*gldap.Entry{
				gldap.NewEntry("uid=alice,dc=example,dc=com", nil),
			},
		},
		bindErrors: map[string]error{
			"uid=alice,dc=example,dc=com": errors.New("upstream said alice-password"),
		},
	}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), basicHeader("alice"))
	if err == nil {
		t.Fatal("AuthenticateAuthorization error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid credentials") ||
		strings.Contains(err.Error(), "alice-password") {
		t.Fatalf("error = %q, want sanitized invalid credentials", err)
	}
}

func TestProviderAuthenticateAuthorizationSanitizesSearchFailure(t *testing.T) {
	conn := &fakeConn{
		searchErr: errors.New("ldap backend leaked uid=alice alice-password dc=example,dc=com"),
	}
	provider, err := NewProvider(Config{
		URL:          "ldaps://ldap.example.com",
		BaseDN:       "dc=example,dc=com",
		DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
		Dialer:       &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), basicHeader("alice"))
	if err == nil {
		t.Fatal("AuthenticateAuthorization error = nil, want error")
	}
	if !strings.Contains(err.Error(), "search user failed") ||
		strings.Contains(err.Error(), "alice") ||
		strings.Contains(err.Error(), "alice-password") ||
		strings.Contains(err.Error(), "dc=example") {
		t.Fatalf("error = %q, want sanitized search failure", err)
	}
}

func TestNewProviderRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "unsupported scheme",
			cfg: Config{
				URL:          "https://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "scheme",
		},
		{
			name: "url query is rejected",
			cfg: Config{
				URL:          "ldaps://ldap.example.com:636?bind_password=secret",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "query",
		},
		{
			name: "url fragment is rejected",
			cfg: Config{
				URL:          "ldaps://ldap.example.com:636#secret",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "fragment",
		},
		{
			name: "missing role mapping",
			cfg: Config{
				URL:    "ldaps://ldap.example.com",
				BaseDN: "dc=example,dc=com",
			},
			want: "role",
		},
		{
			name: "bind pair mismatch",
			cfg: Config{
				URL:          "ldaps://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				BindDN:       "cn=openclarion,dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "bind dn",
		},
		{
			name: "bind dn whitespace",
			cfg: Config{
				URL:          "ldaps://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				BindDN:       " cn=openclarion,dc=example,dc=com ",
				BindPassword: "service-password",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "bind dn",
		},
		{
			name: "bind password line break",
			cfg: Config{
				URL:          "ldaps://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				BindDN:       "cn=openclarion,dc=example,dc=com",
				BindPassword: "service-password\n",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "bind password",
		},
		{
			name: "bad filter",
			cfg: Config{
				URL:          "ldaps://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				UserFilter:   "(uid=alice)",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "{username}",
		},
		{
			name: "plaintext ldap requires explicit transport choice",
			cfg: Config{
				URL:          "ldap://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
			},
			want: "start tls",
		},
		{
			name: "start tls is not layered on ldaps",
			cfg: Config{
				URL:          "ldaps://ldap.example.com",
				BaseDN:       "dc=example,dc=com",
				DefaultRoles: []ports.AuthRole{ports.AuthRoleOwner},
				StartTLS:     true,
			},
			want: "start tls",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.cfg)
			if err == nil {
				t.Fatal("NewProvider error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked raw LDAP URL component: %q", err.Error())
			}
		})
	}
}

func basicHeader(username string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":alice-password"))
}

type fakeDialer struct {
	conn Conn
	err  error
	urls []string
}

func (d *fakeDialer) DialURL(ctx context.Context, rawURL string) (Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.urls = append(d.urls, rawURL)
	if d.err != nil {
		return nil, d.err
	}
	return d.conn, nil
}

type bindCall struct {
	username string
	password string
}

type fakeConn struct {
	bindErrors     map[string]error
	binds          []bindCall
	closed         bool
	operations     []string
	searchErr      error
	searches       []*gldap.SearchRequest
	searchResult   *gldap.SearchResult
	startTLSErr    error
	startTLSCalls  int
	startTLSConfig *tls.Config
}

func (c *fakeConn) Bind(username, password string) error {
	c.operations = append(c.operations, "bind:"+username)
	c.binds = append(c.binds, bindCall{username: username, password: password})
	if c.bindErrors != nil {
		if err := c.bindErrors[username]; err != nil {
			return err
		}
	}
	return nil
}

func (c *fakeConn) Search(searchRequest *gldap.SearchRequest) (*gldap.SearchResult, error) {
	c.operations = append(c.operations, "search")
	c.searches = append(c.searches, searchRequest)
	if c.searchErr != nil {
		return nil, c.searchErr
	}
	if c.searchResult == nil {
		return &gldap.SearchResult{}, nil
	}
	return c.searchResult, nil
}

func (c *fakeConn) StartTLS(config *tls.Config) error {
	c.operations = append(c.operations, "start_tls")
	c.startTLSCalls++
	c.startTLSConfig = config
	if c.startTLSErr != nil {
		return c.startTLSErr
	}
	return nil
}

func (c *fakeConn) Close() {
	c.closed = true
}
