// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

// Files and directories with the extended attribute user.duplicacy_exclude are excluded.  The attributes are
// listed with their namespace, so the check used to look for a name that never appears.
func excludedByAttribute(attirbutes map[string][]byte) bool {
	_, ok := attirbutes["user.duplicacy_exclude"]
	if !ok {
		_, ok = attirbutes["duplicacy_exclude"]
	}
	return ok
}
