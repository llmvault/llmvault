const SILENT_TOOL_CALL_THRESHOLD: u32 = 3;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProgressReminder {
    pub silent_tool_calls: u32,
    pub message: String,
}

#[derive(Debug, Default)]
pub struct ProgressReminderDetector {
    silent_tool_calls: u32,
}

impl ProgressReminderDetector {
    pub fn observe_response(
        &mut self,
        visible_text: &str,
        tool_call_count: usize,
    ) -> Option<ProgressReminder> {
        if !visible_text.trim().is_empty() {
            self.silent_tool_calls = 0;
            return None;
        }
        if tool_call_count == 0 {
            return None;
        }

        self.silent_tool_calls = self
            .silent_tool_calls
            .saturating_add(tool_call_count.min(u32::MAX as usize) as u32);
        if self.silent_tool_calls < SILENT_TOOL_CALL_THRESHOLD {
            return None;
        }

        let silent_tool_calls = self.silent_tool_calls;
        self.silent_tool_calls = 0;
        Some(ProgressReminder {
            silent_tool_calls,
            message: progress_reminder_message(silent_tool_calls),
        })
    }
}

fn progress_reminder_message(silent_tool_calls: u32) -> String {
    format!(
        "You have called {silent_tool_calls} tool calls without providing a user-facing summary. \
         Before calling more tools, emit a concise progress update to the user in one to two short \
         paragraphs summarizing what you are doing, what you have learned, and what you will do next."
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reminds_after_three_silent_tool_calls() {
        let mut detector = ProgressReminderDetector::default();

        assert!(detector.observe_response("", 1).is_none());
        assert!(detector.observe_response("   ", 1).is_none());
        let reminder = detector
            .observe_response("", 1)
            .expect("third silent tool call should remind");

        assert_eq!(reminder.silent_tool_calls, 3);
        assert!(reminder.message.contains("3 tool calls"));
        assert!(reminder.message.contains("progress update"));
        assert!(reminder.message.contains("one to two short paragraphs"));
        assert!(!reminder.message.contains("three to five"));
    }

    #[test]
    fn visible_text_resets_silent_count() {
        let mut detector = ProgressReminderDetector::default();

        assert!(detector.observe_response("", 2).is_none());
        assert!(detector
            .observe_response("I am checking the repo.", 0)
            .is_none());
        assert!(detector.observe_response("", 1).is_none());
        assert!(detector.observe_response("", 1).is_none());
        assert!(detector.observe_response("", 1).is_some());
    }

    #[test]
    fn counts_parallel_tool_calls_as_silent_work() {
        let mut detector = ProgressReminderDetector::default();

        let reminder = detector
            .observe_response("", 3)
            .expect("parallel silent tool calls should remind");

        assert_eq!(reminder.silent_tool_calls, 3);
    }
}
