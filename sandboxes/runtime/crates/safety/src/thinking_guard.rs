use domain::OverthinkingConfig;
use regex::Regex;
use std::time::Instant;

#[derive(Clone)]
pub struct ThinkingGuard {
    think_tag_re: Regex,
    orphan_closing_re: Regex,
}

impl Default for ThinkingGuard {
    fn default() -> Self {
        Self::new()
    }
}

impl ThinkingGuard {
    pub fn new() -> Self {
        Self {
            think_tag_re: Regex::new(r"(?is)<\s*think\s*>.*?<\s*/\s*think\s*>").unwrap(),
            orphan_closing_re: Regex::new(r"(?i)<\s*/\s*think\s*>").unwrap(),
        }
    }

    pub fn strip_thinking(&self, text: &str) -> (String, bool) {
        let mut stripped = text.to_string();
        let mut had_tags = false;

        let cleaned = self.think_tag_re.replace_all(text, "");
        if cleaned != text {
            had_tags = true;
            stripped = cleaned.to_string();
        }

        if let Some(m) = self.orphan_closing_re.find(&stripped) {
            had_tags = true;
            stripped = stripped[m.end()..].to_string();
        }

        (stripped, had_tags)
    }

    pub fn extract_thinking_content(&self, text: &str) -> Option<String> {
        let captures: Vec<_> = self.think_tag_re.find_iter(text).collect();
        if captures.is_empty() {
            return None;
        }

        let thinking: Vec<&str> = captures.iter().map(|m| m.as_str()).collect();
        Some(thinking.join("\n"))
    }
}

/// Streaming `<think>…</think>` filter that maintains state across deltas of a
/// single turn. Stripping per delta (the old `strip_thinking` approach) leaks
/// reasoning whenever an opening or closing tag is split across two deltas
/// (e.g. `"…<thi"` + `"nk>secret"`), because neither delta contains a complete
/// tag. This is a character state machine: it tracks whether we are currently
/// inside a think block and buffers a trailing run of characters that could be
/// the prefix of a tag until the next delta resolves it.
///
/// Tag matching is case-insensitive and tolerant of internal whitespace
/// (`< think >`, `</ THINK >`), matching `ThinkingGuard`'s regexes.
#[derive(Debug, Default)]
pub struct ThinkingStreamFilter {
    in_think: bool,
    /// Buffered trailing characters that are a viable prefix of the tag we are
    /// scanning for (`<think>` when outside, `</think>` when inside). Held back
    /// from output until proven not to be a tag.
    pending: String,
}

/// Result of feeding one delta to the [`ThinkingStreamFilter`].
#[derive(Debug, Default, PartialEq, Eq)]
pub struct ThinkingSplit {
    /// Text outside any think block — safe to surface to the user.
    pub visible: String,
    /// Reasoning text captured from inside think blocks (tags stripped).
    pub thinking: String,
    /// Whether any think-tag boundary was observed while processing this delta.
    pub had_thinking: bool,
}

impl ThinkingStreamFilter {
    pub fn new() -> Self {
        Self::default()
    }

    /// Feed the next streamed delta. Returns the visible (non-thinking) text and
    /// any reasoning text seen in this delta, with state carried forward so tags
    /// split across deltas are handled correctly. A trailing partial tag is held
    /// in `pending` and not emitted until the following delta (or `finish`).
    pub fn push(&mut self, delta: &str) -> ThinkingSplit {
        let mut out = ThinkingSplit::default();
        // Work over the already-buffered prefix plus the new delta so a tag that
        // straddles the boundary is matched as one unit.
        let mut work = std::mem::take(&mut self.pending);
        work.push_str(delta);
        let chars: Vec<char> = work.chars().collect();
        let mut i = 0;
        while i < chars.len() {
            // Outside a block we scan for an opening `<think>`; inside, a closing
            // `</think>`.
            let scanning_open = !self.in_think;
            match match_tag_at(&chars, i, scanning_open) {
                TagMatch::Complete(consumed) => {
                    self.in_think = !self.in_think;
                    out.had_thinking = true;
                    i += consumed;
                }
                TagMatch::Partial => {
                    // The remaining tail is a viable tag prefix; buffer it and
                    // wait for the next delta to disambiguate.
                    self.pending = chars[i..].iter().collect();
                    return out;
                }
                TagMatch::None => {
                    if self.in_think {
                        out.thinking.push(chars[i]);
                    } else {
                        out.visible.push(chars[i]);
                    }
                    i += 1;
                }
            }
        }
        out
    }

    /// Flush any buffered partial tag at the end of the turn. A dangling prefix
    /// that never completed into a real tag must be surfaced (as visible text if
    /// we were outside a block, or as reasoning if inside) so no content is lost.
    pub fn finish(&mut self) -> ThinkingSplit {
        let mut out = ThinkingSplit::default();
        let pending = std::mem::take(&mut self.pending);
        if pending.is_empty() {
            return out;
        }
        if self.in_think {
            out.thinking.push_str(&pending);
        } else {
            out.visible.push_str(&pending);
        }
        out
    }
}

enum TagMatch {
    /// A complete tag was matched; the value is the number of chars consumed.
    Complete(usize),
    /// The remaining input is a (possibly empty after `<`) viable prefix of the
    /// expected tag but is not yet complete — wait for more input.
    Partial,
    /// Not a tag at this position.
    None,
}

/// Attempt to match an opening (`<think>`) or closing (`</think>`) tag starting
/// at `chars[start]`, tolerating internal whitespace and any case. Returns
/// whether the match is complete, a viable partial prefix (needs more input), or
/// no match at all.
fn match_tag_at(chars: &[char], start: usize, opening: bool) -> TagMatch {
    if chars[start] != '<' {
        return TagMatch::None;
    }
    // Expected literal sequence (ignoring optional whitespace between tokens).
    // Opening: < think > ; Closing: < / think >
    let tokens: &[&str] = if opening {
        &["think", ">"]
    } else {
        &["/", "think", ">"]
    };
    let mut idx = start + 1; // consumed '<'
    for (t, token) in tokens.iter().enumerate() {
        // Optional whitespace before each token.
        while idx < chars.len() && chars[idx].is_whitespace() {
            idx += 1;
        }
        if idx >= chars.len() {
            return TagMatch::Partial;
        }
        // The `>` terminator is a single char; the word tokens match case-insensitively.
        if *token == ">" {
            if chars[idx] == '>' {
                idx += 1;
                // Matched all tokens including terminator.
                if t == tokens.len() - 1 {
                    return TagMatch::Complete(idx - start);
                }
            } else {
                return TagMatch::None;
            }
        } else if *token == "/" {
            if chars[idx] == '/' {
                idx += 1;
            } else {
                return TagMatch::None;
            }
        } else {
            // Match the word token (e.g. "think") char-by-char, case-insensitively.
            for token_ch in token.chars() {
                if idx >= chars.len() {
                    return TagMatch::Partial;
                }
                if !chars[idx].eq_ignore_ascii_case(&token_ch) {
                    return TagMatch::None;
                }
                idx += 1;
            }
        }
    }
    TagMatch::Complete(idx - start)
}

pub struct OverthinkingDetector {
    thinking_start: Option<Instant>,
    thinking_token_count: u64,
    last_content_change_at: u64,
    last_content_hash: u64,
    config: OverthinkingConfig,
}

impl OverthinkingDetector {
    pub fn new(config: OverthinkingConfig) -> Self {
        Self {
            thinking_start: None,
            thinking_token_count: 0,
            last_content_change_at: 0,
            last_content_hash: 0,
            config,
        }
    }

    pub fn feed(&mut self, thinking_token: &str) -> OverthinkingStatus {
        if self.thinking_start.is_none() {
            self.thinking_start = Some(Instant::now());
        }

        self.thinking_token_count += 1;

        if let Some(start) = self.thinking_start {
            let elapsed = start.elapsed().as_secs();
            if elapsed >= self.config.max_duration_secs {
                return OverthinkingStatus::TimeLimitExceeded {
                    duration_secs: elapsed,
                    tokens: self.thinking_token_count,
                };
            }
        }

        if self.thinking_token_count >= self.config.max_tokens {
            return OverthinkingStatus::TokenLimitExceeded {
                tokens: self.thinking_token_count,
            };
        }

        let hash = hash_tail(thinking_token, 50);
        if hash != self.last_content_hash {
            self.last_content_hash = hash;
            self.last_content_change_at = self.thinking_token_count;
        } else {
            let stalled_for = self.thinking_token_count - self.last_content_change_at;
            if stalled_for >= self.config.stall_threshold {
                return OverthinkingStatus::Stalled { stalled_for };
            }
        }

        OverthinkingStatus::Ok
    }

    pub fn reset(&mut self) {
        self.thinking_start = None;
        self.thinking_token_count = 0;
        self.last_content_change_at = 0;
        self.last_content_hash = 0;
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OverthinkingStatus {
    Ok,
    TimeLimitExceeded { duration_secs: u64, tokens: u64 },
    TokenLimitExceeded { tokens: u64 },
    Stalled { stalled_for: u64 },
}

impl OverthinkingStatus {
    pub fn is_overthinking(&self) -> bool {
        !matches!(self, Self::Ok)
    }

    pub fn reason(&self) -> String {
        match self {
            Self::Ok => String::new(),
            Self::TimeLimitExceeded {
                duration_secs,
                tokens,
            } => format!("Thinking exceeded {duration_secs}s time limit ({tokens} tokens emitted)"),
            Self::TokenLimitExceeded { tokens } => {
                format!("Thinking exceeded {tokens} token limit")
            }
            Self::Stalled { stalled_for } => {
                format!("Thinking stalled — {stalled_for} tokens without new content")
            }
        }
    }
}

fn hash_tail(text: &str, n: usize) -> u64 {
    use std::hash::{Hash, Hasher};
    let tail = if text.len() > n {
        &text[text.len() - n..]
    } else {
        text
    };
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    tail.hash(&mut hasher);
    hasher.finish()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strips_complete_think_block() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("<think>step 1\nstep 2</think>final answer");
        assert_eq!(result, "final answer");
        assert!(had_tags);
    }

    #[test]
    fn handles_whitespace_in_tags() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("< think >reasoning< / think >actual output");
        assert_eq!(result, "actual output");
        assert!(had_tags);
    }

    #[test]
    fn strips_orphan_closing_tag() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("stale reasoning</think>final answer");
        assert_eq!(result, "final answer");
        assert!(had_tags);
    }

    #[test]
    fn passes_clean_text_unchanged() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("regular output without tags");
        assert_eq!(result, "regular output without tags");
        assert!(!had_tags);
    }

    #[test]
    fn preserves_clean_delta_boundary_whitespace() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking(" visible delta ");
        assert_eq!(result, " visible delta ");
        assert!(!had_tags);
    }

    #[test]
    fn extracts_thinking_content() {
        let guard = ThinkingGuard::new();
        let thinking = guard.extract_thinking_content("<think>step 1\nstep 2</think>final");
        assert_eq!(thinking, Some("<think>step 1\nstep 2</think>".to_string()));
    }

    #[test]
    fn returns_none_when_no_think_tags() {
        let guard = ThinkingGuard::new();
        let thinking = guard.extract_thinking_content("plain text");
        assert_eq!(thinking, None);
    }

    /// Feed a sequence of deltas through the stateful filter and collect the
    /// concatenated visible + thinking output plus whether any tag was seen.
    fn run_filter(deltas: &[&str]) -> (String, String, bool) {
        let mut filter = ThinkingStreamFilter::new();
        let mut visible = String::new();
        let mut thinking = String::new();
        let mut had = false;
        for delta in deltas {
            let split = filter.push(delta);
            visible.push_str(&split.visible);
            thinking.push_str(&split.thinking);
            had |= split.had_thinking;
        }
        let tail = filter.finish();
        visible.push_str(&tail.visible);
        thinking.push_str(&tail.thinking);
        had |= tail.had_thinking;
        (visible, thinking, had)
    }

    #[test]
    fn stream_filter_strips_whole_block_in_one_delta() {
        let (visible, thinking, had) = run_filter(&["<think>secret plan</think>final answer"]);
        assert_eq!(visible, "final answer");
        assert_eq!(thinking, "secret plan");
        assert!(had);
    }

    #[test]
    fn stream_filter_handles_opening_tag_split_across_deltas() {
        // The opening tag is split: "<thi" + "nk>secret</think>visible".
        // A per-delta stripper would leak "secret" because neither delta has a
        // complete tag.
        let (visible, thinking, had) = run_filter(&["before <thi", "nk>secret", "</think>after"]);
        assert_eq!(visible, "before after");
        assert_eq!(thinking, "secret");
        assert!(had);
    }

    #[test]
    fn stream_filter_handles_closing_tag_split_across_deltas() {
        let (visible, thinking, _) = run_filter(&["<think>reasoning</thi", "nk>answer"]);
        assert_eq!(visible, "answer");
        assert_eq!(thinking, "reasoning");
    }

    #[test]
    fn stream_filter_handles_single_char_deltas() {
        let chars: Vec<&str> = "a<think>x</think>b".split("").filter(|s| !s.is_empty()).collect();
        let (visible, thinking, had) = run_filter(&chars);
        assert_eq!(visible, "ab");
        assert_eq!(thinking, "x");
        assert!(had);
    }

    #[test]
    fn stream_filter_passes_clean_text_unchanged() {
        let (visible, thinking, had) = run_filter(&["just ", "a normal ", "answer"]);
        assert_eq!(visible, "just a normal answer");
        assert!(thinking.is_empty());
        assert!(!had);
    }

    #[test]
    fn stream_filter_emits_dangling_lt_as_visible() {
        // A lone "<" that never becomes a tag (e.g. "a < b") must not be dropped.
        let (visible, _thinking, had) = run_filter(&["a < b is true"]);
        assert_eq!(visible, "a < b is true");
        assert!(!had);
    }

    #[test]
    fn stream_filter_flushes_dangling_partial_tag_at_finish() {
        // A turn that ends mid-tag must surface the buffered prefix, not lose it.
        let (visible, _thinking, _had) = run_filter(&["answer<thi"]);
        assert_eq!(visible, "answer<thi");
    }

    #[test]
    fn stream_filter_tolerates_whitespace_and_case_in_tags() {
        let (visible, thinking, had) = run_filter(&["a< THINK >r</ Think >b"]);
        assert_eq!(visible, "ab");
        assert_eq!(thinking, "r");
        assert!(had);
    }

    #[test]
    fn overthinking_detector_time_limit() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 0,
            max_tokens: 10000,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        let status = detector.feed("test");
        assert_eq!(
            status,
            OverthinkingStatus::TimeLimitExceeded {
                duration_secs: 0,
                tokens: 1
            }
        );
    }

    #[test]
    fn overthinking_detector_token_limit() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 2,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        assert_eq!(detector.feed("a"), OverthinkingStatus::Ok);
        assert_eq!(
            detector.feed("b"),
            OverthinkingStatus::TokenLimitExceeded { tokens: 2 }
        );
    }

    #[test]
    fn overthinking_detector_stall_detection() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 10000,
            stall_threshold: 3,
        };
        let mut detector = OverthinkingDetector::new(config);

        detector.feed("unique content 1");
        detector.feed("unique content 2");
        detector.feed("unique content 3");

        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(
            detector.feed("same"),
            OverthinkingStatus::Stalled { stalled_for: 3 }
        );
    }

    #[test]
    fn overthinking_detector_reset() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 10,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        detector.feed("a");
        detector.feed("b");
        detector.feed("c");
        detector.reset();
        assert_eq!(detector.feed("d"), OverthinkingStatus::Ok);
        assert_eq!(detector.thinking_token_count, 1);
    }
}
