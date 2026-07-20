#!/usr/bin/env python3

import importlib.util
import random
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("staging-session-load.py")
SPEC = importlib.util.spec_from_file_location("staging_session_load", SCRIPT)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot import {SCRIPT}")
LOAD_TEST = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = LOAD_TEST
SPEC.loader.exec_module(LOAD_TEST)


class BurstPlanTests(unittest.TestCase):
    def test_default_plan_is_two_coordinated_waves(self) -> None:
        plan, gap = LOAD_TEST.build_burst_plan(
            sessions=100,
            first_wave_size=50,
            min_rounds=3,
            max_rounds=5,
            gap_min=3,
            gap_max=5,
            rng=random.Random(1234),
        )

        self.assertEqual(len(plan), 100)
        self.assertTrue(3 <= gap <= 5)
        self.assertEqual({row["wave"] for row in plan[:50]}, {1})
        self.assertEqual({row["start_offset_seconds"] for row in plan[:50]}, {0.0})
        self.assertEqual({row["wave"] for row in plan[50:]}, {2})
        self.assertEqual({row["start_offset_seconds"] for row in plan[50:]}, {gap})
        self.assertTrue(all(3 <= row["rounds_planned"] <= 5 for row in plan))

    def test_messages_cycle_through_expected_characters(self) -> None:
        values = [LOAD_TEST.burst_message(index) for index in range(5)]

        self.assertEqual([value[0] for value in values], ["A", "B", "C", "D", "A"])
        self.assertEqual(values[0][1], "This is a test - reply only with character A")

    def test_exact_character_reply_rejects_extra_model_output(self) -> None:
        self.assertTrue(LOAD_TEST.exact_character_reply(" A\n", "A"))
        self.assertFalse(LOAD_TEST.exact_character_reply("A.", "A"))
        self.assertFalse(LOAD_TEST.exact_character_reply("`A`", "A"))


class BurstAssertionTests(unittest.TestCase):
    def row(self, index: int) -> dict:
        return {
            "index": index,
            "agent_id": f"agent-{index}",
            "session_id": f"session-{index}",
            "rounds_planned": 1,
            "rounds": [
                {
                    "accepted": True,
                    "response": {"outcome": "responded", "exact_match": True},
                }
            ],
            "sandbox_after_idle_wait": {"status": "stopped"},
            "control_after_idle_wait": {"status": "stopped"},
        }

    def test_all_assertions_require_both_sleep_sources(self) -> None:
        rows = [self.row(1), self.row(2)]

        self.assertTrue(all(LOAD_TEST.burst_assertions(rows, 2).values()))
        rows[1]["control_after_idle_wait"]["status"] = "running"
        assertions = LOAD_TEST.burst_assertions(rows, 2)
        self.assertTrue(assertions["all_sandboxes_asleep"])
        self.assertFalse(assertions["all_control_sandboxes_stopped"])


class ControlPlaneProjectionTests(unittest.TestCase):
    def test_projects_go_default_json_field_names(self) -> None:
        runner = LOAD_TEST.project_runner(
            {
                "ID": "runner-id",
                "Name": "runner0",
                "Status": "healthy",
                "ReservedCPU": 42,
            }
        )

        self.assertEqual(runner["id"], "runner-id")
        self.assertEqual(runner["name"], "runner0")
        self.assertEqual(runner["status"], "healthy")
        self.assertEqual(runner["reserved_cpu"], 42)

    def test_json_field_accepts_snake_case_or_go_name(self) -> None:
        self.assertEqual(LOAD_TEST.json_field({"Status": "stopped"}, "status", "Status"), "stopped")
        self.assertEqual(LOAD_TEST.json_field({"status": "running"}, "status", "Status"), "running")

    def test_http_meta_keeps_failure_body_for_diagnostics(self) -> None:
        result = LOAD_TEST.HTTPResult(
            status=503,
            duration_ms=12.5,
            body={"error": "capacity unavailable"},
            error="HTTP 503",
        )

        self.assertEqual(
            LOAD_TEST.http_meta(result)["response_body"],
            {"error": "capacity unavailable"},
        )


class ParserTests(unittest.TestCase):
    def test_burst_defaults_match_the_requested_benchmark(self) -> None:
        args = LOAD_TEST.build_parser().parse_args(
            ["burst", "--runner-host", "runner0=192.0.2.1"]
        )

        self.assertEqual(args.sessions, 100)
        self.assertEqual(args.first_wave_size, 50)
        self.assertEqual((args.wave_gap_min, args.wave_gap_max), (3.0, 5.0))
        self.assertEqual((args.min_messages, args.max_messages), (3, 5))
        self.assertEqual((args.idle_wait_min, args.idle_wait_max), (20.0, 25.0))
        self.assertEqual(args.sandbox_size, "nano")
        self.assertEqual(args.model, "mimo-v2.5-pro")


if __name__ == "__main__":
    unittest.main()
