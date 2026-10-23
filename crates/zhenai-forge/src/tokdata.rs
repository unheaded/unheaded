// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

//! Pre-tokenized training and eval data: one JSON object per line,
//! `{"tokens": [int, ...], "answer_start": int}`.

use serde::Deserialize;

/// One pre-tokenized example.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TokExample {
    pub tokens: Vec<u32>,
    pub answer_start: Option<usize>,
}

/// Why a data file was refused.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DataError {
    Malformed {
        line: usize,
        reason: String,
    },
    MissingAnswerStart {
        line: usize,
    },
    AnswerStartOutOfRange {
        line: usize,
        answer_start: usize,
        len: usize,
    },
    TooShort {
        line: usize,
        len: usize,
    },
    Empty,
}

impl std::fmt::Display for DataError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DataError::Malformed { line, reason } => write!(
                f,
                "line {line}: not a {{tokens, answer_start}} record: {reason}"
            ),
            DataError::MissingAnswerStart { line } => write!(f, "line {line}: no answer_start"),
            DataError::AnswerStartOutOfRange {
                line,
                answer_start,
                len,
            } => {
                write!(
                    f,
                    "line {line}: answer_start {answer_start} is outside 1..{len} (tokens: {len})"
                )
            }
            DataError::TooShort { line, len } => {
                write!(f, "line {line}: {len} token(s); at least 2 are needed")
            }
            DataError::Empty => write!(f, "no examples"),
        }
    }
}

impl std::error::Error for DataError {}

/// What a load kept and what it did not.
#[derive(Debug, Default)]
pub struct LoadReport {
    pub examples: Vec<TokExample>,
    /// (line number, why) for every non-empty line not kept.
    pub dropped: Vec<(usize, String)>,
    /// Kept examples with no answer_start (the caller's fallback applies).
    pub missing_answer_start: usize,
}

#[derive(Deserialize)]
struct Lenient {
    tokens: Vec<u32>,
    #[serde(default)]
    answer_start: Option<usize>,
}

/// The strict record: exactly these two keys, both required.
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Strict {
    tokens: Vec<u32>,
    answer_start: Option<usize>,
}

/// Parse a pre-tokenized JSONL file. Line numbers are 1-based and count
/// blank lines.
///
/// `strict` (`--strict-data`): the first problem is an error naming its
/// line: an unparseable non-empty line, a key other than `tokens` and
/// `answer_start`, a missing `answer_start`, one outside
/// `1..tokens.len()`, or fewer than 2 tokens. This closes the WAVE14 H6
/// class: nothing is dropped silently and no example falls back to the
/// CLI `--answer-start`.
///
/// Not strict (old corpora): unparseable and too-short lines are dropped,
/// and each is listed in `dropped`; examples without `answer_start` are
/// counted in `missing_answer_start`. The caller must print both.
pub fn parse_tokenized_jsonl(content: &str, strict: bool) -> Result<LoadReport, DataError> {
    let mut report = LoadReport::default();
    for (i, raw) in content.lines().enumerate() {
        let line = i + 1;
        if raw.trim().is_empty() {
            continue;
        }
        let parsed = if strict {
            serde_json::from_str::<Strict>(raw).map(|s| (s.tokens, s.answer_start))
        } else {
            serde_json::from_str::<Lenient>(raw).map(|s| (s.tokens, s.answer_start))
        };
        let (tokens, answer_start) = match parsed {
            Ok(v) => v,
            Err(e) if strict => {
                return Err(DataError::Malformed {
                    line,
                    reason: e.to_string(),
                })
            }
            Err(e) => {
                report.dropped.push((line, e.to_string()));
                continue;
            }
        };
        if tokens.len() < 2 {
            if strict {
                return Err(DataError::TooShort {
                    line,
                    len: tokens.len(),
                });
            }
            report
                .dropped
                .push((line, format!("{} token(s)", tokens.len())));
            continue;
        }
        match answer_start {
            None if strict => return Err(DataError::MissingAnswerStart { line }),
            None => report.missing_answer_start += 1,
            Some(a) if strict && (a == 0 || a >= tokens.len()) => {
                return Err(DataError::AnswerStartOutOfRange {
                    line,
                    answer_start: a,
                    len: tokens.len(),
                });
            }
            Some(_) => {}
        }
        report.examples.push(TokExample {
            tokens,
            answer_start,
        });
    }
    if report.examples.is_empty() {
        return Err(DataError::Empty);
    }
    Ok(report)
}

/// Print what a non-strict load dropped or will default, so it is never
/// silent. `what` names the file's role ("training data").
pub fn print_load_warnings(what: &str, report: &LoadReport, cli_answer_start: usize) {
    if !report.dropped.is_empty() {
        let first: Vec<String> = report
            .dropped
            .iter()
            .take(5)
            .map(|(n, why)| format!("line {n}: {why}"))
            .collect();
        eprintln!(
            "  WARNING: {} line(s) of {what} dropped (pass --strict-data to refuse the file instead): {}{}",
            report.dropped.len(),
            first.join("; "),
            if report.dropped.len() > 5 { "; ..." } else { "" }
        );
    }
    if report.missing_answer_start > 0 {
        eprintln!(
            "  WARNING: {} example(s) of {what} have no answer_start and use --answer-start {cli_answer_start} \
             (the WAVE14 H6 class; pass --strict-data to refuse them)",
            report.missing_answer_start
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strict_data_malformed_line_is_an_error_with_its_number() {
        let data = "{\"tokens\":[1,2,3],\"answer_start\":1}\n\n{\"tokens\":[1,2,\n";
        let err = parse_tokenized_jsonl(data, true).unwrap_err();
        assert!(
            matches!(err, DataError::Malformed { line: 3, .. }),
            "{err:?}"
        );
        assert!(err.to_string().starts_with("line 3: "), "{err}");
    }

    #[test]
    fn strict_data_missing_answer_start_is_an_error() {
        let data = "{\"tokens\":[1,2,3],\"answer_start\":1}\n{\"tokens\":[4,5,6]}\n";
        assert_eq!(
            parse_tokenized_jsonl(data, true).unwrap_err(),
            DataError::MissingAnswerStart { line: 2 }
        );
    }

    #[test]
    fn strict_data_reads_tokens_and_answer_start_exactly() {
        // The H6 shape: answer_start must be a field, never a token.
        let r = parse_tokenized_jsonl("{\"tokens\":[1,2,107],\"answer_start\":2}\n", true).unwrap();
        assert_eq!(
            r.examples,
            vec![TokExample {
                tokens: vec![1, 2, 107],
                answer_start: Some(2)
            }]
        );
        assert!(r.dropped.is_empty());
    }

    #[test]
    fn strict_data_h6_fixture_out_of_range_is_refused() {
        let data = "{\"tokens\":[1,2,107],\"answer_start\":360}\n";
        assert_eq!(
            parse_tokenized_jsonl(data, true).unwrap_err(),
            DataError::AnswerStartOutOfRange {
                line: 1,
                answer_start: 360,
                len: 3
            }
        );
        let zero = "{\"tokens\":[1,2,107],\"answer_start\":0}\n";
        assert!(matches!(
            parse_tokenized_jsonl(zero, true),
            Err(DataError::AnswerStartOutOfRange { .. })
        ));
    }

    #[test]
    fn strict_data_extra_keys_and_short_sequences_are_refused() {
        let extra = "{\"tokens\":[1,2,3],\"answer_start\":1,\"question\":\"x\"}\n";
        assert!(matches!(
            parse_tokenized_jsonl(extra, true),
            Err(DataError::Malformed { line: 1, .. })
        ));
        let short = "{\"tokens\":[1],\"answer_start\":1}\n";
        assert_eq!(
            parse_tokenized_jsonl(short, true).unwrap_err(),
            DataError::TooShort { line: 1, len: 1 }
        );
    }

    #[test]
    fn strict_data_off_still_reports_what_it_dropped() {
        let data = "{\"tokens\":[1,2,3],\"answer_start\":1}\nnot json\n{\"tokens\":[1]}\n{\"tokens\":[7,8,9]}\n";
        let r = parse_tokenized_jsonl(data, false).unwrap();
        assert_eq!(r.examples.len(), 2);
        assert_eq!(
            r.dropped.iter().map(|(n, _)| *n).collect::<Vec<_>>(),
            vec![2, 3]
        );
        assert_eq!(r.missing_answer_start, 1);
    }

    #[test]
    fn strict_data_empty_file_is_an_error_either_way() {
        assert_eq!(
            parse_tokenized_jsonl("\n\n", true).unwrap_err(),
            DataError::Empty
        );
        assert_eq!(
            parse_tokenized_jsonl("junk\n", false).unwrap_err(),
            DataError::Empty
        );
    }
}
