// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package zhenrag

import (
	"regexp"
	"strings"
)

// Verb is one row of the destructive-verb table. The prompts and the gate
// read the same row, at two strictnesses:
//
//   - The prompts are broad on purpose. A retrieved reference that so much as
//     mentions `rm`, `delete` or `format` makes the chat model refuse to
//     recite it (the A1 source-poison probe, eval/coding-gate/probe-2026-05-02).
//   - The gate (Champion Rule 3) refuses a tool call outright, even after a
//     human confirms, so it matches only unambiguous forms: "delete" and
//     "format" appear in harmless arguments every day, and "rm" alone would
//     refuse removing a scratch file. Those rows have no Pattern.
type Verb struct {
	// Chat is the verb as listed in the Chat and Web prompts ("" = not listed).
	Chat string
	// Agent is the verb as listed in the Agent prompt ("" = not listed).
	Agent string
	// Pattern is this verb's alternative in DestructivePattern ("" = the
	// gate does not match it; see the type comment). Word boundaries keep
	// "rebootstrap" from matching "reboot".
	Pattern string
	// Seeds are inputs Pattern must match (and, for prompt-only rows, notes
	// of what the gate deliberately lets through). Tests run every seed.
	Seeds []string
}

// Verbs is the destructive-verb table. Order matters for the prompts: each
// prompt lists its verbs in table order, and the goldens fix that order.
// New gate-only rows go at the end.
var Verbs = []Verb{
	{Chat: "rm"}, // prompt-only: plain `rm file` is routine
	{Chat: "rm -rf", Agent: "rm -rf", Pattern: `rm\s+-[rf]+\b`, Seeds: []string{"rm -rf /tmp/x", "rm -f foo", "rm -r dir"}},
	{Chat: "delete", Agent: "delete"}, // prompt-only: kanban "delete", SQL DELETE … are routine
	{Chat: "drop table", Agent: "drop table", Pattern: `\bdrop\s+table\b`, Seeds: []string{"DROP TABLE users"}},
	{Chat: "wipe", Agent: "wipe", Pattern: `\bwipe\b`, Seeds: []string{"wipe the disk"}},
	{Chat: "format", Agent: "format"}, // prompt-only: "format-string", "date format"
	{Chat: "mkfs", Agent: "mkfs", Pattern: `\bmkfs(\.[a-z0-9]+)?\b`, Seeds: []string{"mkfs /dev/sdb1", "mkfs.ext4 /dev/sdb1", "mkfs.xfs -f /dev/vdb"}},
	{Chat: "dd", Agent: "dd if=", Pattern: `\bdd\s+(if|of)=`, Seeds: []string{"dd if=/dev/zero of=/dev/sda", "dd of=/dev/sda"}},
	{Chat: "> /dev/", Agent: "> /dev/", Pattern: `>\s*/dev/(sd[a-z]|vd[a-z]|xvd[a-z]|hd[a-z]|nvme[0-9]|mmcblk[0-9])`, Seeds: []string{"cat x > /dev/sda", "echo >/dev/nvme0n1", "cat img > /dev/mmcblk0"}},
	{Chat: "chmod 000", Agent: "chmod 000", Pattern: `\bchmod\s+(-R\s+)?000\b`, Seeds: []string{"chmod 000 /etc", "chmod -R 000 /srv"}},
	{Chat: "shutdown", Agent: "shutdown", Pattern: `\bshutdown\b`, Seeds: []string{"shutdown -h now"}},
	{Chat: "reboot", Agent: "reboot", Pattern: `\breboot\b`, Seeds: []string{"sudo reboot"}},
	{Chat: "kill -9", Agent: "kill -9", Pattern: `\bkill\s+-9\b`, Seeds: []string{"kill -9 1"}},
	{Chat: "truncate", Agent: "truncate", Pattern: `\btruncate\b`, Seeds: []string{"truncate -s 0 /var/log/syslog", "TRUNCATE audit_events"}},
	{Chat: "unlink", Agent: "unlink", Pattern: `\bunlink\b`, Seeds: []string{"unlink /etc/passwd"}},
	{Chat: "git push --force", Agent: "git push --force", Pattern: `git\s+push\s+--force\b`, Seeds: []string{"git push --force origin main"}},
	{Chat: "git reset --hard", Agent: "git reset --hard", Pattern: `git\s+reset\s+--hard\b`, Seeds: []string{"git reset --hard HEAD~3"}},

	// Gate-only rows: not named in any prompt (adding them there would
	// change the prompt bytes the coding gate measured).
	{Pattern: `rm\s+/`, Seeds: []string{"rm /etc/passwd"}},
	{Pattern: `\bdrop\s+database\b`, Seeds: []string{"DROP DATABASE unheaded_app"}},
	// RT-07 (sweep-security 2026-10-27): disk and file destroyers the
	// original list missed.
	{Pattern: `\bsrm\b`, Seeds: []string{"srm -r ~/secrets"}},
	{Pattern: `\bwipefs\b`, Seeds: []string{"wipefs -a /dev/sdb"}},
	{Pattern: `\bshred\b`, Seeds: []string{"shred -u key.pem"}},
	{Pattern: `\bblkdiscard\b`, Seeds: []string{"blkdiscard /dev/nvme0n1"}},
	{Pattern: `\bluks(Format|Erase)\b`, Seeds: []string{"cryptsetup luksFormat /dev/sdb2", "cryptsetup luksErase /dev/sdb2"}},
	{Pattern: `\bnvme\s+format\b`, Seeds: []string{"nvme format /dev/nvme0n1"}},
	{Pattern: `\bs?fdisk\b`, Seeds: []string{"fdisk /dev/sda", "sfdisk /dev/sda < layout"}},
	{Pattern: `\bparted\b`, Seeds: []string{"parted /dev/sda rm 1"}},
	{Pattern: `\bdmsetup\s+(remove|remove_all|wipe_table|clear)\b`, Seeds: []string{"dmsetup remove_all", "dmsetup wipe_table vg-lv"}},
}

// DestructivePattern matches any destructive shell-verb form in a string.
// pkg/champion applies it to every string-valued tool-call argument (Rule 3:
// refused regardless of source trust, even after confirmation). (?i) makes
// it case-insensitive.
var DestructivePattern = regexp.MustCompile(destructivePatternSource())

func destructivePatternSource() string {
	alts := make([]string, 0, len(Verbs))
	for _, v := range Verbs {
		if v.Pattern != "" {
			alts = append(alts, v.Pattern)
		}
	}
	return `(?i)` + strings.Join(alts, "|")
}

// ChatVerbs is the verb list the Chat and Web prompts name, in table order.
func ChatVerbs() []string { return pick(func(v Verb) string { return v.Chat }) }

// AgentVerbs is the verb list the Agent prompt names, in table order.
func AgentVerbs() []string { return pick(func(v Verb) string { return v.Agent }) }

func pick(f func(Verb) string) []string {
	var out []string
	for _, v := range Verbs {
		if s := f(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// DestructiveVerbsProse renders a verb list the way the prompts name it:
// each verb in backticks, comma-separated.
func DestructiveVerbsProse(verbs []string) string {
	return "`" + strings.Join(verbs, "`, `") + "`"
}
