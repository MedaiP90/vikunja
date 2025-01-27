// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public Licensee as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public Licensee for more details.
//
// You should have received a copy of the GNU Affero General Public Licensee
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package ldap

import (
	"fmt"
	"strings"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/modules/auth"
	"code.vikunja.io/api/pkg/user"

	"github.com/go-ldap/ldap/v3"
	"xorm.io/xorm"
)

func ConnectAndBindToLDAPDirectory() (l *ldap.Conn, err error) {
	if !config.AuthLdapEnabled.GetBool() {
		return
	}

	var protocol = "ldap"
	if config.AuthLdapUseTLS.GetBool() {
		protocol = "ldaps"
	}
	url := fmt.Sprintf(
		"%s://%s:%d",
		protocol,
		config.AuthLdapHost.GetString(),
		config.AuthLdapPort.GetInt(),
	)
	l, err = ldap.DialURL(url)
	if err != nil {
		log.Fatalf("Could not connect to LDAP server: %s", err)
	}

	err = l.Bind(
		config.AuthLdapBindDN.GetString(),
		config.AuthLdapBindPassword.GetString(),
	)
	return
}

func AuthenticateUserInLDAP(s *xorm.Session, username, password string) (u *user.User, err error) {
	if password == "" || username == "" {
		return nil, user.ErrNoUsernamePassword{}
	}

	l, err := ConnectAndBindToLDAPDirectory()
	if err != nil {
		log.Errorf("Could not bind to LDAP server: %s", err)
		return
	}
	defer l.Close()

	log.Debugf("Connected to LDAP server")

	searchRequest := ldap.NewSearchRequest(
		config.AuthLdapBaseDN.GetString(),
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=inetOrgPerson)(uid=%s))", username),
		[]string{
			"dn",
			config.AuthLdapAttributeUsername.GetString(),
			config.AuthLdapAttributeEmail.GetString(),
			config.AuthLdapAttributeDisplayname.GetString(),
		},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return
	}

	if len(sr.Entries) > 1 || len(sr.Entries) == 0 {
		log.Debugf("Found %d entries for username %s", len(sr.Entries), username)
		return nil, user.ErrWrongUsernameOrPassword{}
	}

	userdn := sr.Entries[0].DN

	// Bind as the user to verify their password
	err = l.Bind(userdn, password)
	if err != nil {
		return
	}

	return getOrCreateLdapUser(s, sr.Entries[0])
}

func getOrCreateLdapUser(s *xorm.Session, entry *ldap.Entry) (u *user.User, err error) {
	username := entry.GetAttributeValue(config.AuthLdapAttributeUsername.GetString())
	email := entry.GetAttributeValue(config.AuthLdapAttributeEmail.GetString())
	name := entry.GetAttributeValue(config.AuthLdapAttributeDisplayname.GetString())

	u, err = user.GetUserWithEmail(s, &user.User{
		Issuer:  user.IssuerLDAP,
		Subject: username,
	})
	if err != nil && !user.IsErrUserDoesNotExist(err) {
		return nil, err
	}

	// If no user exists, create one with the preferred username if it is not already taken
	if user.IsErrUserDoesNotExist(err) {
		uu := &user.User{
			Username: strings.ReplaceAll(username, " ", "-"),
			Email:    email,
			Name:     name,
			Status:   user.StatusActive,
			Issuer:   user.IssuerLDAP,
			Subject:  username,
		}

		return auth.CreateUserWithRandomUsername(s, uu)
	}

	return
}
