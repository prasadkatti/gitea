// Copyright 2021 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package smtp

import (
	"errors"
	"net/smtp"
	"net/textproto"
	"strings"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/mailer"
)

// Authenticate queries if the provided login/password is authenticates against the SMTP server
// Users will be autoregistered as required
func (source *Source) Authenticate(user *models.User, login, password string) (*models.User, error) {
	// Verify allowed domains.
	if len(source.AllowedDomains) > 0 {
		idx := strings.Index(login, "@")
		if idx == -1 {
			return nil, models.ErrUserNotExist{Name: login}
		} else if !util.IsStringInSlice(login[idx+1:], strings.Split(source.AllowedDomains, ","), true) {
			return nil, models.ErrUserNotExist{Name: login}
		}
	}

	var auth smtp.Auth
	switch source.Auth {
	case PlainAuthentication:
		auth = smtp.PlainAuth("", login, password, source.Host)
	case LoginAuthentication:
		auth = &loginAuthenticator{login, password}
	case CRAMMD5Authentication:
		auth = smtp.CRAMMD5Auth(login, password)
	default:
		return nil, errors.New("unsupported SMTP auth type")
	}

	if err := Authenticate(auth, source); err != nil {
		// Check standard error format first,
		// then fallback to worse case.
		tperr, ok := err.(*textproto.Error)
		if (ok && tperr.Code == 535) ||
			strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, models.ErrUserNotExist{Name: login}
		}
		if (ok && tperr.Code == 534) ||
			strings.Contains(err.Error(), "Application-specific password required") {
			return nil, models.ErrUserNotExist{Name: login}
		}
		return nil, err
	}

	if user != nil {
		return user, nil
	}

	username := login
	idx := strings.Index(login, "@")
	if idx > -1 {
		username = login[:idx]
	}

	user = &models.User{
		LowerName:   strings.ToLower(username),
		Name:        strings.ToLower(username),
		Email:       login,
		Passwd:      password,
		LoginType:   models.LoginSMTP,
		LoginSource: source.loginSource.ID,
		LoginName:   login,
		IsActive:    true,
	}

	if err := models.CreateUser(user); err != nil {
		return user, err
	}

	mailer.SendRegisterNotifyMail(user)

	return user, nil
}
