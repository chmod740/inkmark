package main

import _ "embed"

// thirdPartyNotices contains the attribution and license terms for the
// third-party components distributed with InkMark.
//
//go:embed THIRD_PARTY_NOTICES.txt
var thirdPartyNotices string

// GetThirdPartyNotices returns the notices embedded in this application build.
// Keeping the notices in the binary makes them available without network access.
func (a *App) GetThirdPartyNotices() string {
	return thirdPartyNotices
}
