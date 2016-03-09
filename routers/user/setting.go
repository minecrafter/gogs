// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"io/ioutil"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTINGS_PROFILE      base.TplName = "user/settings/profile"
	SETTINGS_PASSWORD     base.TplName = "user/settings/password"
	SETTINGS_EMAILS       base.TplName = "user/settings/email"
	SETTINGS_SSH_KEYS     base.TplName = "user/settings/sshkeys"
	SETTINGS_SOCIAL       base.TplName = "user/settings/social"
	SETTINGS_APPLICATIONS base.TplName = "user/settings/applications"
	SETTINGS_DELETE       base.TplName = "user/settings/delete"
	SETTINGS_TWOFACTOR    base.TplName = "user/settings/two_factor"
	NOTIFICATION          base.TplName = "user/notification"
	TWOFACTOR_ENROLL      base.TplName = "user/settings/two_factor_enroll"
)

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsProfile"] = true
	ctx.HTML(200, SETTINGS_PROFILE)
}

func handleUsernameChange(ctx *middleware.Context, newName string) {
	// Non-local users are not allowed to change their username.
	if len(newName) == 0 || !ctx.User.IsLocal() {
		return
	}

	// Check if user name has been changed
	if ctx.User.LowerName != strings.ToLower(newName) {
		if err := models.ChangeUserName(ctx.User, newName); err != nil {
			switch {
			case models.IsErrUserAlreadyExist(err):
				ctx.Flash.Error(ctx.Tr("newName_been_taken"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings")
			case models.IsErrEmailAlreadyUsed(err):
				ctx.Flash.Error(ctx.Tr("form.email_been_used"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings")
			case models.IsErrNameReserved(err):
				ctx.Flash.Error(ctx.Tr("user.newName_reserved"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings")
			case models.IsErrNamePatternNotAllowed(err):
				ctx.Flash.Error(ctx.Tr("user.newName_pattern_not_allowed"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings")
			default:
				ctx.Handle(500, "ChangeUserName", err)
			}
			return
		}
		log.Trace("User name changed: %s -> %s", ctx.User.Name, newName)
	}

	// In case it's just a case change
	ctx.User.Name = newName
	ctx.User.LowerName = strings.ToLower(newName)
}

func SettingsPost(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsProfile"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_PROFILE)
		return
	}

	handleUsernameChange(ctx, form.Name)
	if ctx.Written() {
		return
	}

	ctx.User.FullName = form.FullName
	ctx.User.Email = form.Email
	ctx.User.Website = form.Website
	ctx.User.Location = form.Location
	if len(form.Gravatar) > 0 {
		ctx.User.Avatar = base.EncodeMD5(form.Gravatar)
		ctx.User.AvatarEmail = form.Gravatar
	}
	if err := models.UpdateUser(ctx.User); err != nil {
		ctx.Handle(500, "UpdateUser", err)
		return
	}

	log.Trace("User settings updated: %s", ctx.User.Name)
	ctx.Flash.Success(ctx.Tr("settings.update_profile_success"))
	ctx.Redirect(setting.AppSubUrl + "/user/settings")
}

// FIXME: limit size.
func UpdateAvatarSetting(ctx *middleware.Context, form auth.UploadAvatarForm, ctxUser *models.User) error {
	ctxUser.UseCustomAvatar = form.Enable

	if form.Avatar != nil {
		fr, err := form.Avatar.Open()
		if err != nil {
			return fmt.Errorf("Avatar.Open: %v", err)
		}
		defer fr.Close()

		data, err := ioutil.ReadAll(fr)
		if err != nil {
			return fmt.Errorf("ioutil.ReadAll: %v", err)
		}
		if _, ok := base.IsImageFile(data); !ok {
			return errors.New(ctx.Tr("settings.uploaded_avatar_not_a_image"))
		}
		if err = ctxUser.UploadAvatar(data); err != nil {
			return fmt.Errorf("UploadAvatar: %v", err)
		}
	} else {
		// No avatar is uploaded but setting has been changed to enable,
		// generate a random one when needed.
		if form.Enable && !com.IsFile(ctxUser.CustomAvatarPath()) {
			if err := ctxUser.GenerateRandomAvatar(); err != nil {
				log.Error(4, "GenerateRandomAvatar[%d]: %v", ctxUser.Id, err)
			}
		}
	}

	if err := models.UpdateUser(ctxUser); err != nil {
		return fmt.Errorf("UpdateUser: %v", err)
	}

	return nil
}

func SettingsAvatar(ctx *middleware.Context, form auth.UploadAvatarForm) {
	if err := UpdateAvatarSetting(ctx, form, ctx.User); err != nil {
		ctx.Flash.Error(err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("settings.update_avatar_success"))
	}

	ctx.Redirect(setting.AppSubUrl + "/user/settings")
}

func SettingsDeleteAvatar(ctx *middleware.Context) {
	if err := ctx.User.DeleteAvatar(); err != nil {
		ctx.Flash.Error(err.Error())
	}

	ctx.Redirect(setting.AppSubUrl + "/user/settings")
}

func SettingsPassword(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsPassword"] = true
	ctx.HTML(200, SETTINGS_PASSWORD)
}

func SettingsPasswordPost(ctx *middleware.Context, form auth.ChangePasswordForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsPassword"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_PASSWORD)
		return
	}

	if !ctx.User.ValidatePassword(form.OldPassword) {
		ctx.Flash.Error(ctx.Tr("settings.password_incorrect"))
	} else if form.Password != form.Retype {
		ctx.Flash.Error(ctx.Tr("form.password_not_match"))
	} else {
		ctx.User.Passwd = form.Password
		ctx.User.Salt = models.GetUserSalt()
		ctx.User.EncodePasswd()
		if err := models.UpdateUser(ctx.User); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
		log.Trace("User password updated: %s", ctx.User.Name)
		ctx.Flash.Success(ctx.Tr("settings.change_password_success"))
	}

	ctx.Redirect(setting.AppSubUrl + "/user/settings/password")
}

func SettingsEmails(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsEmails"] = true

	emails, err := models.GetEmailAddresses(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "GetEmailAddresses", err)
		return
	}
	ctx.Data["Emails"] = emails

	ctx.HTML(200, SETTINGS_EMAILS)
}

func SettingsEmailPost(ctx *middleware.Context, form auth.AddEmailForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsEmails"] = true

	// Make emailaddress primary.
	if ctx.Query("_method") == "PRIMARY" {
		if err := models.MakeEmailPrimary(&models.EmailAddress{ID: ctx.QueryInt64("id")}); err != nil {
			ctx.Handle(500, "MakeEmailPrimary", err)
			return
		}

		log.Trace("Email made primary: %s", ctx.User.Name)
		ctx.Redirect(setting.AppSubUrl + "/user/settings/email")
		return
	}

	// Add Email address.
	emails, err := models.GetEmailAddresses(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "GetEmailAddresses", err)
		return
	}
	ctx.Data["Emails"] = emails

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_EMAILS)
		return
	}

	e := &models.EmailAddress{
		UID:         ctx.User.Id,
		Email:       form.Email,
		IsActivated: !setting.Service.RegisterEmailConfirm,
	}
	if err := models.AddEmailAddress(e); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), SETTINGS_EMAILS, &form)
			return
		}
		ctx.Handle(500, "AddEmailAddress", err)
		return
	}

	// Send confirmation e-mail
	if setting.Service.RegisterEmailConfirm {
		mailer.SendActivateEmailMail(ctx.Context, ctx.User, e)

		if err := ctx.Cache.Put("MailResendLimit_"+ctx.User.LowerName, ctx.User.LowerName, 180); err != nil {
			log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
		}
		ctx.Flash.Info(ctx.Tr("settings.add_email_confirmation_sent", e.Email, setting.Service.ActiveCodeLives/60))
	} else {
		ctx.Flash.Success(ctx.Tr("settings.add_email_success"))
	}

	log.Trace("Email address added: %s", e.Email)
	ctx.Redirect(setting.AppSubUrl + "/user/settings/email")
}

func DeleteEmail(ctx *middleware.Context) {
	if err := models.DeleteEmailAddress(&models.EmailAddress{ID: ctx.QueryInt64("id")}); err != nil {
		ctx.Handle(500, "DeleteEmail", err)
		return
	}
	log.Trace("Email address deleted: %s", ctx.User.Name)

	ctx.Flash.Success(ctx.Tr("settings.email_deletion_success"))
	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/user/settings/email",
	})
}

func SettingsSSHKeys(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsSSHKeys"] = true

	keys, err := models.ListPublicKeys(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ListPublicKeys", err)
		return
	}
	ctx.Data["Keys"] = keys

	ctx.HTML(200, SETTINGS_SSH_KEYS)
}

func SettingsSSHKeysPost(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsSSHKeys"] = true

	keys, err := models.ListPublicKeys(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ListPublicKeys", err)
		return
	}
	ctx.Data["Keys"] = keys

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_SSH_KEYS)
		return
	}

	content, err := models.CheckPublicKeyString(form.Content)
	if err != nil {
		if models.IsErrKeyUnableVerify(err) {
			ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
		} else {
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
			ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
			return
		}
	}

	if _, err = models.AddPublicKey(ctx.User.Id, form.Title, content); err != nil {
		ctx.Data["HasError"] = true
		switch {
		case models.IsErrKeyAlreadyExist(err):
			ctx.Data["Err_Content"] = true
			ctx.RenderWithErr(ctx.Tr("settings.ssh_key_been_used"), SETTINGS_SSH_KEYS, &form)
		case models.IsErrKeyNameAlreadyUsed(err):
			ctx.Data["Err_Title"] = true
			ctx.RenderWithErr(ctx.Tr("settings.ssh_key_name_used"), SETTINGS_SSH_KEYS, &form)
		default:
			ctx.Handle(500, "AddPublicKey", err)
		}
		return
	}

	ctx.Flash.Success(ctx.Tr("settings.add_key_success", form.Title))
	ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
}

func DeleteSSHKey(ctx *middleware.Context) {
	if err := models.DeletePublicKey(ctx.User, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeletePublicKey: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("settings.ssh_key_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/user/settings/ssh",
	})
}

func SettingsApplications(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsApplications"] = true

	tokens, err := models.ListAccessTokens(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ListAccessTokens", err)
		return
	}
	ctx.Data["Tokens"] = tokens

	ctx.HTML(200, SETTINGS_APPLICATIONS)
}

func SettingsApplicationsPost(ctx *middleware.Context, form auth.NewAccessTokenForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsApplications"] = true

	if ctx.HasError() {
		tokens, err := models.ListAccessTokens(ctx.User.Id)
		if err != nil {
			ctx.Handle(500, "ListAccessTokens", err)
			return
		}
		ctx.Data["Tokens"] = tokens
		ctx.HTML(200, SETTINGS_APPLICATIONS)
		return
	}

	t := &models.AccessToken{
		UID:  ctx.User.Id,
		Name: form.Name,
	}
	if err := models.NewAccessToken(t); err != nil {
		ctx.Handle(500, "NewAccessToken", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("settings.generate_token_succees"))
	ctx.Flash.Info(t.Sha1)

	ctx.Redirect(setting.AppSubUrl + "/user/settings/applications")
}

func SettingsDeleteApplication(ctx *middleware.Context) {
	if err := models.DeleteAccessTokenByID(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteAccessTokenByID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("settings.delete_token_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/user/settings/applications",
	})
}

func SettingsDelete(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsDelete"] = true

	if ctx.Req.Method == "POST" {
		if _, err := models.UserSignIn(ctx.User.Name, ctx.Query("password")); err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_password"), SETTINGS_DELETE, nil)
			} else {
				ctx.Handle(500, "UserSignIn", err)
			}
			return
		}

		if err := models.DeleteUser(ctx.User); err != nil {
			switch {
			case models.IsErrUserOwnRepos(err):
				ctx.Flash.Error(ctx.Tr("form.still_own_repo"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/delete")
			case models.IsErrUserHasOrgs(err):
				ctx.Flash.Error(ctx.Tr("form.still_has_org"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/delete")
			default:
				ctx.Handle(500, "DeleteUser", err)
			}
		} else {
			log.Trace("Account deleted: %s", ctx.User.Name)
			ctx.Redirect(setting.AppSubUrl + "/")
		}
		return
	}

	ctx.HTML(200, SETTINGS_DELETE)
}

func SettingsTwoFactor(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsTwoFactor"] = true

	twofa, err := models.GetTwoFactorByUID(ctx.User.Id)
	if err != nil {
		// Not having two factor should not be fatal.
		if !models.IsErrTwoFactorNotExist(err) {
			ctx.Handle(500, "SettingsTwoFactor", err)
			return
		}
	}
	ctx.Data["Has2FA"] = twofa != nil

	ctx.HTML(200, SETTINGS_TWOFACTOR)
}

func SettingsEnrollTwoFactor(ctx *middleware.Context) {
	twofa, err := models.GetTwoFactorByUID(ctx.User.Id)
	if err != nil {
		// Not having two factor should not be fatal.
		if !models.IsErrTwoFactorNotExist(err) {
			ctx.Handle(500, "SettingsEnrollTwoFactor", err)
			return
		}
	}

	if twofa != nil {
		// TODO: Need to handle the case where 2FA is enabled
		return
	}

	// check if we already have TOTP
	var key *otp.Key
	urlInSession := ctx.Session.Get("twofaUrl")
	if urlInSession != nil {
		if url, ok := urlInSession.(string); ok {
			// We will reuse
			key, _ = otp.NewKeyFromURL(url)
			// We'll just generate a new key instead if the deserialization fails
		}
	}

	if key == nil {
		// generate TOTP secret
		opts := totp.GenerateOpts{
			Issuer: setting.AppName,
			AccountName: ctx.User.Name,
		}
		key, err = totp.Generate(opts)
		if err != nil {
			ctx.Handle(500, "SettingsEnrollTwoFactor", err)
			return
		}
	}

	// Generate an image
	var output bytes.Buffer
	image, err := key.Image(200, 200)
	if err != nil {
		ctx.Handle(500, "SettingsEnrollTwoFactor", err)
		return
	}

	err = png.Encode(&output, image)
	if err != nil {
		ctx.Handle(500, "SettingsEnrollTwoFactor", err)
		return
	}

	imgUrl := "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes())
	imgTag := "<img src=\"" + imgUrl + "\" alt=\"QR code\">"
	ctx.Session.Set("twofaUrl", key.String())

	ctx.Data["QRCodeImg"] = template.HTML(imgTag)
	ctx.Data["Secret"] = key.Secret()

	ctx.HTML(200, TWOFACTOR_ENROLL)
}

func SettingsEnrollTwoFactorPost(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsSettingsTwoFactor"] = true

	twofa, err := models.GetTwoFactorByUID(ctx.User.Id)
	if err != nil {
		// Not having two factor should not be fatal.
		if !models.IsErrTwoFactorNotExist(err) {
			ctx.Handle(500, "SettingsEnrollTwoFactor", err)
			return
		}
	}

	if twofa != nil {
		ctx.Flash.Error(ctx.Tr("settings.two_factor_already_active_error"))
		return
	}

	// check if we already have TOTP
	urlInSession := ctx.Session.Get("twofaUrl")
	if urlInSession != nil {
		if url, ok := urlInSession.(string); ok {
			// We will reuse
			key, err := otp.NewKeyFromURL(url)
			if err != nil {
				ctx.Handle(500, "SettingsEnrollTwoFactorPost", err)
				return
			}

			code := ctx.Query("code")
			if totp.Validate(code, key.Secret()) {
				// Validation successful, register the authentication.
				twofa := models.TwoFactor{
					UID: ctx.User.Id,
					Secret: key.Secret(),
				}

				if err = models.NewTwoFactor(&twofa); err != nil {
					ctx.Handle(500, "SettingsEnrollTwoFactorPost", err)
					return
				}

				ctx.Session.Delete("twofaUrl")
				ctx.Flash.Success(ctx.Tr("settings.two_factor_enroll_success"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/twofa")
			} else {
				ctx.Flash.Error(ctx.Tr("settings.two_factor_enroll_error_code"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/twofa/enroll")
			}
		} else {
			ctx.Handle(500, "SettingsEnrollTwoFactorPost", err)
			return
		}
	}
}

func SettingsDisableTwoFactor(ctx *middleware.Context) {
	if _, err := models.UserSignIn(ctx.User.Name, ctx.Query("password")); err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Flash.Error(ctx.Tr("form.enterred_invalid_password"))
		} else {
			ctx.Flash.Error("UserSignIn: " + err.Error())
		}

		ctx.Redirect(setting.AppSubUrl + "/user/settings/twofa")
	} else {
		twofa, err := models.GetTwoFactorByUID(ctx.User.Id)
		if err != nil {
			if models.IsErrTwoFactorNotExist(err) {
				ctx.Flash.Error(ctx.Tr("settings.two_factor_not_active_error"))
			} else {
				ctx.Flash.Error("GetTwoFactorByUID: " + err.Error())
			}

			ctx.Redirect(setting.AppSubUrl + "/user/settings/twofa")
			return
		}

		err = models.DeleteTwoFactorByID(twofa.ID)
		if err != nil {
			ctx.Flash.Error("DeleteTwoFactorByID: " + err.Error())
		} else {
			ctx.Flash.Success(ctx.Tr("settings.two_factor_deactivated"))
		}

		ctx.Redirect(setting.AppSubUrl + "/user/settings/twofa")
	}
}
