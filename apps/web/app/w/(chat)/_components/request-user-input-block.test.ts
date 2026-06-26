import { describe, expect, it } from "vitest"
import {
  buildDismissalAnswerPayload,
  buildQuestionAnswerPayload,
  initialAnswerDraft,
} from "@/app/w/(chat)/_components/request-user-input-block"
import type { RequestUserInputQuestion } from "@/app/w/(chat)/_lib/static-data"

const questions: RequestUserInputQuestion[] = [
  {
    id: "deployment_path",
    header: "Deploy",
    question: "Which deployment path should Codex use?",
    options: [
      { label: "Ship It", description: "Deploy immediately." },
      { label: "Hold", description: "Wait for review." },
    ],
  },
]

describe("request user input payload helpers", () => {
  it("defaults each question to its first option", () => {
    expect(initialAnswerDraft(questions)).toEqual({
      deployment_path: { label: "Ship It", other: undefined },
    })
  })

  it("builds the runtime question answer payload", () => {
    expect(
      buildQuestionAnswerPayload(questions, {
        deployment_path: { label: "Hold" },
      })
    ).toEqual({
      answers: {
        deployment_path: { answers: ["Hold"] },
      },
    })
  })

  it("requires text when Other is selected", () => {
    expect(
      buildQuestionAnswerPayload(questions, {
        deployment_path: { label: "Other", other: "" },
      })
    ).toBeNull()
    expect(
      buildQuestionAnswerPayload(questions, {
        deployment_path: { label: "Other", other: "Use staging first" },
      })
    ).toEqual({
      answers: {
        deployment_path: {
          answers: ["Other"],
          other: "Use staging first",
        },
      },
    })
  })

  it("builds an Other answer for dismissed requests", () => {
    expect(buildDismissalAnswerPayload(questions)).toEqual({
      answers: {
        deployment_path: {
          answers: ["Other"],
          other: "Dismissed by user.",
        },
      },
    })
  })
})
