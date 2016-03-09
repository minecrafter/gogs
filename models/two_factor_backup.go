// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"
)

// TwoFactorBackup represents a stored two-factor authentication backup code.
type TwoFactorBackup struct {
	ID                int64 `xorm:"pk autoincr"`
	UID               int64 `xorm:"UNIQUE"`
	Code              string    `xorm:"UNIQUE VARCHAR(16)"`
	Created           time.Time `xorm:"CREATED"`
	Updated           time.Time
  Burnt             bool
}

// NewTwoFactorBackup creates a new two factor token.
func NewTwoFactorBackup(t *TwoFactorBackup) error {
	_, err := x.Insert(t)
	return err
}

// BurnTwoFactorBackupCode will try to "burn" a two-factor authentication backup code.
func BurnTwoFactorBackupCode(uid int64, code string) error {
  t := &TwoFactorBackup{UID: uid, Code: code, Burnt: false}
  has, err := x.Get(t)
  if err != nil {
    return err
  } else if !has {
    return nil, ErrTwoFactorNotExist{uid}
  }

  // Now we burn the backup code
  t.Burnt = true
  return UpdateTwoFactorBackup(t)
}

// UpdateTwoFactorBackup updates information of two factor token.
func UpdateTwoFactorBackup(t *TwoFactorBackup) error {
	_, err := x.Id(t.ID).AllCols().Update(t)
	return err
}

// DeleteTwoFactorCodesFromUser deletes two factor authentication backup codes from a user.
func DeleteTwoFactorCodesFromUser(uid int64) error {
  deleteBy := &TwoFactorBackup{UID: uid}
	_, err := x.Delete(deleteBy)
	return err
}
