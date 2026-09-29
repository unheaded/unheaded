// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import "unheaded/services/wotan/internal/topicpattern"

// MatchTopic reports whether topic matches pattern; see topicpattern.Match.
// The HTTP API uses the same matcher, so both transports agree.
func MatchTopic(pattern, topic string) bool {
	return topicpattern.Match(pattern, topic)
}

// FilterTopics returns the topics that match pattern; see topicpattern.
func FilterTopics(pattern string, topics []string) []string {
	return topicpattern.FilterTopics(pattern, topics)
}
