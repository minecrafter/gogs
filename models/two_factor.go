// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"
)

// TwoFactor represents a stored two-factor authentication secret.
type TwoFactor struct {
	ID                int64 `xorm:"pk autoincr"`
	UID               int64 `xorm:"UNIQUE"`
	Secret            string    `xorm:"UNIQUE VARCHAR(16)"`
	Created           time.Time `xorm:"CREATED"`
	Updated           time.Time
}

// NewTwoFactor creates a new two factor token.
func NewTwoFactor(t *TwoFactor) error {
	_, err := x.Insert(t)
	return err
}

// GetTwoFactorByUID returns two factor token by given user ID.
func GetTwoFactorByUID(uid int64) (*TwoFactor, error) {
	t := &TwoFactor{UID: uid}
	has, err := x.Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTwoFactorNotExist{uid}
	}
	return t, nil
}

// UpdateTwoFactorupdates information of two factor token.
func UpdateTwoFactor(t *TwoFactor) error {
	_, err := x.Id(t.ID).AllCols().Update(t)
	return err
}

// DeleteTwoFactorByID deletes two faactor token by given ID.
func DeleteTwoFactorByID(id int64) error {
	_, err := x.Id(id).Delete(new(TwoFactor))
	return err
}
