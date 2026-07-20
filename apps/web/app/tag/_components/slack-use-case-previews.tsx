"use client"

import { Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import {
  AnimatedAgentReply,
  Composer,
  easeOut,
  Message,
  people,
  reveal,
  slackFont,
  SlackChrome,
  SlackMention,
} from "./slack-previews"

export function SlackMemoryPreview() {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div
        initial="hidden"
        whileInView="show"
        viewport={{ once: true, amount: 0.35 }}
        className="mx-auto w-full max-w-[590px] overflow-hidden rounded-[9px] border border-[#d6d6d6] bg-[#fefefe] shadow-sm"
        style={slackFont}
      >
        <div className="flex h-9 items-center justify-center bg-[#350d36] px-3">
          <div className="flex h-5 w-3/5 items-center justify-center gap-2 rounded-[4px] bg-[#5d3d5e] px-2 text-[9px] text-[#eee7ee]">
            <AppIcon icon="search" size={10} /> Search Hivy workspace
          </div>
        </div>
        <div className="flex h-11 items-center justify-between border-b border-[#dddddd] px-4">
          <span className="text-[12px] font-bold"># product-support</span>
          <span className="text-[9px] text-[#616061]">Two weeks later</span>
        </div>
        <div className="py-2">
          <motion.div variants={reveal} custom={0.3}>
            <Message person={people.maya} time="11:08 AM">
              <SlackMention /> we’re changing the importer again. What did we
              learn from the last failure?
            </Message>
          </motion.div>
          <AnimatedAgentReply time="11:09 AM" delay={1.7}>
            Keep the workspace lookup list-aware. Last time, the job treated a
            multi-workspace response as one record and broke every later step.
          </AnimatedAgentReply>
        </div>
      </motion.div>
    </MotionConfig>
  )
}

const teamUseCases = [
  {
    id: "support",
    label: "Customer support",
    icon: "headset",
    title: "Find the failure while the report is still fresh.",
    description:
      "A Support agent can read the report, inspect the connected systems, and return the cause with a fix your team can act on.",
    agent: "Support agent",
    trigger: "Mention in #support-escalations",
    channel: "support-escalations",
    channelDescription: "Customer issues that need investigation",
    channels: [
      "support-triage",
      "support-escalations",
      "customer-voice",
      "engineering",
    ],
    person: people.maya,
    time: "10:42 AM",
    replyTime: "10:43 AM",
    prompt:
      "compare this import error with the last working deploy and tell me what changed.",
    answerLead: "The regression starts in workspace_lookup.",
    answerDetails: [
      "The July 18 deploy changed the response from one workspace to Workspace[].",
      "run_import still reads .id directly. Select the matched workspace before creating the import.",
    ],
    outputs: ["Root cause", "Suggested fix"],
  },
  {
    id: "product",
    label: "Product",
    icon: "search",
    title: "Turn a long feedback thread into a product decision.",
    description:
      "The Product agent can compare what customers said, separate the repeated need from the requested solution, and leave the open question in view.",
    agent: "Product agent",
    trigger: "Mention in #customer-voice",
    channel: "customer-voice",
    channelDescription: "Customer feedback and product requests",
    channels: [
      "product",
      "customer-voice",
      "design-research",
      "product-releases",
    ],
    person: people.leah,
    time: "1:16 PM",
    replyTime: "1:17 PM",
    prompt:
      "turn the export feedback in this thread into a product decision. What repeats, and what should we build first?",
    answerLead: "The repeated need is approval history in exported reports.",
    answerDetails: [
      "Smallest useful scope: add approver, approval time, and final status to CSV and PDF exports.",
      "Decision still needed: whether rejected approvals should include reviewer notes.",
    ],
    outputs: ["Repeated need", "Open decision"],
  },
  {
    id: "finance",
    label: "Finance",
    icon: "chart-spline",
    title: "Explain a cost change before someone builds a spreadsheet.",
    description:
      "The Finance agent can compare usage, schedules, and provider costs, then point to the operating change behind the bill.",
    agent: "Finance agent",
    trigger: "Mention in #finance",
    channel: "finance",
    channelDescription: "Budgets, invoices, and usage reviews",
    channels: ["finance", "billing", "procurement", "leadership"],
    person: people.omar,
    time: "9:05 AM",
    replyTime: "9:06 AM",
    prompt:
      "why did model spend jump this week? Compare usage with last week and call out the driver.",
    answerLead:
      "The increase comes from Support Agent sessions, not higher model prices.",
    answerDetails: [
      "Average tokens per session stayed flat while the number of sessions increased.",
      "The main change is ticket triage running every hour. Moving it to every four hours removes most of the extra runs.",
    ],
    outputs: ["Cost driver", "Recommended change"],
  },
  {
    id: "revenue",
    label: "Revenue",
    icon: "presentation",
    title: "Leave the account thread with the follow-up ready.",
    description:
      "The Revenue agent can bring call notes and the current Slack thread together, identify what still blocks the deal, and draft the next message.",
    agent: "Revenue agent",
    trigger: "Mention in #revenue-ops",
    channel: "revenue-ops",
    channelDescription: "Account follow-ups, handoffs, and deal support",
    channels: ["sales", "revenue-ops", "customer-success", "deal-desk"],
    person: people.jon,
    time: "3:24 PM",
    replyTime: "3:25 PM",
    prompt:
      "prepare the follow-up for this account. Pull the blockers from this thread and our last call.",
    answerLead:
      "Two blockers remain: SSO domain verification and approval-history exports.",
    answerDetails: [
      "Send the security setup guide today. Hold the expansion quote until Product confirms export timing.",
      "Draft reply: I attached the SSO steps and asked Product for the export date. I’ll update you here when it’s confirmed.",
    ],
    outputs: ["Blockers", "Draft follow-up"],
  },
] as const

type TeamUseCase = (typeof teamUseCases)[number]

function TeamSlackScene({ useCase }: { useCase: TeamUseCase }) {
  return (
    <SlackChrome
      focused
      channel={useCase.channel}
      channelDescription={useCase.channelDescription}
      channels={useCase.channels}
    >
      <motion.div
        variants={reveal}
        custom={0.12}
        className="border-b border-[#eeeeee] px-4 py-3"
      >
        <p className="text-[16px] font-bold"># {useCase.channel}</p>
        <p className="mt-1 text-[10px] text-[#616061]">
          Example conversation with the {useCase.agent.toLowerCase()} assigned
          to this channel.
        </p>
      </motion.div>
      <div className="py-2">
        <motion.div variants={reveal} custom={0.35}>
          <Message person={useCase.person} time={useCase.time}>
            <SlackMention /> {useCase.prompt}
          </Message>
        </motion.div>
        <AnimatedAgentReply
          time={useCase.replyTime}
          delay={1.55}
          footer={
            <div className="flex flex-wrap gap-1.5">
              {useCase.outputs.map((output) => (
                <span
                  key={output}
                  className="rounded-[4px] border border-[#dddddd] px-2 py-1 text-[9px] text-[#616061]"
                >
                  {output}
                </span>
              ))}
            </div>
          }
        >
          <p className="font-medium">{useCase.answerLead}</p>
          <ul className="mt-1.5 space-y-1">
            {useCase.answerDetails.map((detail) => (
              <li key={detail} className="flex gap-2">
                <span aria-hidden="true" className="text-[#616061]">
                  •
                </span>
                <span>{detail}</span>
              </li>
            ))}
          </ul>
        </AnimatedAgentReply>
      </div>
      <Composer label={`Message #${useCase.channel}`} />
    </SlackChrome>
  )
}

export function TeamTagUseCases() {
  return (
    <Tabs variant="primary" defaultSelectedKey="support" className="w-full">
      <Tabs.ListContainer className="mx-auto max-w-full overflow-x-auto">
        <Tabs.List
          aria-label="Teams using Hivy Tag"
          className="w-fit min-w-[620px]"
        >
          {teamUseCases.map((useCase) => (
            <Tabs.Tab id={useCase.id} key={useCase.id}>
              <span className="flex items-center justify-center gap-2 whitespace-nowrap">
                <AppIcon icon={useCase.icon} size={15} />
                {useCase.label}
              </span>
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </Tabs.ListContainer>

      {teamUseCases.map((useCase) => (
        <Tabs.Panel id={useCase.id} key={useCase.id} className="mt-9 p-0">
          <motion.div
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.35, ease: easeOut }}
            className="grid min-h-[620px] items-center gap-8 overflow-hidden rounded-sm bg-accent-soft p-4 sm:p-8 lg:grid-cols-[minmax(0,0.72fr)_minmax(240px,0.28fr)] lg:gap-12 lg:p-14"
          >
            <div className="order-2 min-w-0 lg:order-1">
              <TeamSlackScene useCase={useCase} />
            </div>
            <div className="order-1 lg:order-2">
              <h3 className="sr-only">{useCase.title}</h3>
              <p className="sr-only">{useCase.description}</p>
              <div className="rounded-sm bg-foreground p-5 text-background shadow-surface sm:p-6">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-xs font-medium text-background/65">
                    Prompt
                  </span>
                  <span className="inline-flex items-center gap-1.5 text-[0.68rem] text-background/65">
                    <AppIcon icon={useCase.icon} size={13} />
                    {useCase.agent}
                  </span>
                </div>
                <p className="mt-4 text-sm leading-6 text-background/80">
                  <span className="font-medium text-background">@hivy</span>{" "}
                  {useCase.prompt}
                </p>
              </div>
              <p className="mt-4 px-1 text-xs leading-5 text-foreground/60">
                Assigned in #{useCase.channel}. The answer returns to the same
                Slack thread.
              </p>
            </div>
          </motion.div>
        </Tabs.Panel>
      ))}
    </Tabs>
  )
}
