"use client"

import type { ReactNode } from "react"
import { motion, MotionConfig, type Variants } from "motion/react"
import { AppIcon } from "@/components/icon"
import { LogoMark } from "@/components/logo"

const slackFont = {
  fontFamily:
    '"Slack-Lato", "Helvetica Neue", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
}

const people = {
  maya: {
    name: "Maya Chen",
    image:
      "https://images.unsplash.com/photo-1494790108377-be9c29b29330?auto=format&fit=crop&w=96&h=96&q=86",
  },
  leah: {
    name: "Leah Brooks",
    image:
      "https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&w=96&h=96&q=86",
  },
  omar: {
    name: "Omar Bell",
    image:
      "https://images.unsplash.com/photo-1500648767791-00dcc994a43e?auto=format&fit=crop&w=96&h=96&q=86",
  },
  jon: {
    name: "Jon Park",
    image:
      "https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&w=96&h=96&q=86",
  },
} as const

const easeOut = [0.16, 1, 0.3, 1] as const

const reveal: Variants = {
  hidden: { opacity: 0, y: 10 },
  show: (delay = 0) => ({
    opacity: 1,
    y: 0,
    transition: { duration: 0.46, delay, ease: easeOut },
  }),
}

function HivyAvatar({ size = "message" }: { size?: "message" | "small" }) {
  return (
    <span
      className={[
        "flex shrink-0 items-center justify-center overflow-hidden rounded-[5px] bg-[#fff7ef] shadow-[inset_0_0_0_1px_rgba(29,28,29,0.1)]",
        size === "small" ? "size-5 p-[3px]" : "size-9 p-1.5",
      ].join(" ")}
    >
      <LogoMark className="size-full" />
    </span>
  )
}

function PersonAvatar({
  person,
}: {
  person: (typeof people)[keyof typeof people]
}) {
  return (
    <span
      role="img"
      aria-label={`${person.name} profile photo`}
      className="size-9 shrink-0 rounded-[5px] bg-[#e8e8e8] bg-cover bg-center shadow-[inset_0_0_0_1px_rgba(29,28,29,0.08)]"
      style={{ backgroundImage: `url(${person.image})` }}
    />
  )
}

function SlackMention({ children = "@hivy" }: { children?: ReactNode }) {
  return (
    <span className="rounded-[3px] bg-[#e8f5fa] px-1 py-0.5 font-semibold text-[#1264a3]">
      {children}
    </span>
  )
}

function Message({
  person,
  time,
  app = false,
  children,
  footer,
  compact = false,
}: {
  person?: (typeof people)[keyof typeof people]
  time: string
  app?: boolean
  children: ReactNode
  footer?: ReactNode
  compact?: boolean
}) {
  const name = app ? "hivy" : (person?.name ?? "Teammate")

  return (
    <div
      className={[
        "group flex gap-2.5 px-4 hover:bg-[#f8f8f8]",
        compact ? "py-1.5" : "py-2.5",
      ].join(" ")}
    >
      {app ? <HivyAvatar /> : person ? <PersonAvatar person={person} /> : null}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-1.5">
          <span className="text-[13px] leading-4 font-bold text-[#1d1c1d]">
            {name}
          </span>
          {app ? (
            <span className="rounded-[3px] bg-[#e8e8e8] px-1 py-px text-[8px] leading-3 font-bold tracking-[0.04em] text-[#4a4a4a]">
              APP
            </span>
          ) : null}
          <span className="text-[10px] leading-4 text-[#616061]">{time}</span>
        </div>
        <div className="mt-px text-[12.5px] leading-[1.45] text-[#1d1c1d]">
          {children}
        </div>
        {footer ? <div className="mt-2">{footer}</div> : null}
      </div>
    </div>
  )
}

function TypingMessage() {
  return (
    <div className="flex gap-2.5 px-4 py-2.5">
      <HivyAvatar />
      <div>
        <div className="flex items-baseline gap-1.5">
          <span className="text-[13px] leading-4 font-bold text-[#1d1c1d]">
            hivy
          </span>
          <span className="rounded-[3px] bg-[#e8e8e8] px-1 py-px text-[8px] leading-3 font-bold tracking-[0.04em] text-[#4a4a4a]">
            APP
          </span>
        </div>
        <div className="mt-1 flex h-7 w-14 items-center justify-center gap-1 rounded-[12px] border border-[#dddddd] bg-[#f8f8f8]">
          {[0, 1, 2].map((index) => (
            <motion.span
              key={index}
              className="size-1 rounded-full bg-[#616061]"
              animate={{ opacity: [0.35, 1, 0.35], y: [0, -2, 0] }}
              transition={{
                duration: 0.72,
                delay: index * 0.12,
                repeat: Number.POSITIVE_INFINITY,
                ease: "easeInOut",
              }}
            />
          ))}
        </div>
        <p className="mt-1 text-[9px] text-[#616061]">hivy is typing…</p>
      </div>
    </div>
  )
}

function AnimatedAgentReply({
  time,
  children,
  footer,
  delay = 1.65,
}: {
  time: string
  children: ReactNode
  footer?: ReactNode
  delay?: number
}) {
  return (
    <div className="grid [&>*]:[grid-area:1/1]">
      <motion.div
        variants={{
          hidden: { opacity: 0, y: 6 },
          show: {
            opacity: [0, 1, 1, 0],
            y: [6, 0, 0, -3],
            transition: {
              delay: Math.max(0, delay - 0.92),
              duration: 1.18,
              times: [0, 0.15, 0.76, 1],
              ease: easeOut,
            },
          },
        }}
      >
        <TypingMessage />
      </motion.div>
      <motion.div variants={reveal} custom={delay}>
        <Message time={time} app footer={footer}>
          {children}
        </Message>
      </motion.div>
    </div>
  )
}

function Composer({ label }: { label: string }) {
  return (
    <div className="mx-4 mt-auto mb-4 rounded-[7px] border border-[#868686] bg-[#fefefe]">
      <div className="px-3 pt-2.5 text-[11px] text-[#616061]">{label}</div>
      <div className="mt-2 flex h-7 items-center justify-between border-t border-[#eeeeee] px-2.5 text-[#616061]">
        <div className="flex items-center gap-2">
          <AppIcon icon="plus" size={13} />
          <AppIcon icon="type" size={13} />
          <AppIcon icon="message-circle" size={13} />
        </div>
        <AppIcon icon="send" size={13} />
      </div>
    </div>
  )
}

function SlackChrome({
  channel,
  channelDescription,
  activeChannel = channel,
  focused = false,
  children,
}: {
  channel: string
  channelDescription: string
  activeChannel?: string
  focused?: boolean
  children: ReactNode
}) {
  const channels = [
    "announcements",
    "product",
    "product-support",
    "customer-voice",
  ]

  return (
    <MotionConfig reducedMotion="user">
      <motion.div
        role="img"
        aria-label={`Slack workspace showing #${channel}`}
        initial="hidden"
        whileInView="show"
        viewport={{ once: true, amount: 0.28 }}
        className="overflow-hidden rounded-[10px] border border-[#d6d6d6] bg-[#fefefe] text-[#1d1c1d] shadow-[0_18px_50px_rgba(53,13,54,0.14)]"
        style={slackFont}
      >
        <div className="grid h-9 grid-cols-[auto_1fr_auto] items-center gap-3 bg-[#350d36] px-3 text-[#f8f8f8]">
          <AppIcon icon="history" size={13} className="text-[#d7c8d8]" />
          <div className="mx-auto flex h-6 w-full max-w-[360px] items-center justify-center gap-2 rounded-[5px] bg-[#5d3d5e] px-3 text-[10px] text-[#eee7ee] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.16)]">
            <AppIcon icon="search" size={11} />
            Search Hivy workspace
          </div>
          <AppIcon icon="info" size={13} className="text-[#d7c8d8]" />
        </div>

        <div
          className={
            focused
              ? "min-h-[430px]"
              : "grid min-h-[430px] md:grid-cols-[48px_170px_minmax(0,1fr)]"
          }
        >
          {!focused ? (
            <>
              <div className="hidden flex-col items-center gap-2.5 bg-[#3f0e40] py-3 md:flex">
                <span className="flex size-8 items-center justify-center rounded-[8px] bg-[#f8f8f8]">
                  <AppIcon icon="slack" size={18} />
                </span>
                <span className="flex size-8 items-center justify-center rounded-[8px] bg-[#1264a3] text-[9px] font-bold text-[#f8f8f8]">
                  HW
                </span>
                <span className="mt-auto flex size-7 items-center justify-center rounded-full bg-[#583459] text-[#d7c8d8]">
                  <AppIcon icon="plus" size={13} />
                </span>
              </div>

              <aside className="hidden flex-col bg-[#4a154b] text-[#f8f8f8] md:flex">
                <div className="flex h-11 items-center justify-between border-b border-[#633764] px-3">
                  <span className="truncate text-[12px] font-bold">
                    Hivy workspace
                  </span>
                  <AppIcon
                    icon="square-pen"
                    size={14}
                    className="text-[#d7c8d8]"
                  />
                </div>
                <div className="space-y-0.5 px-2 py-2.5 text-[11px] text-[#d7c8d8]">
                  <div className="flex items-center gap-2 rounded-[4px] px-2 py-1">
                    <AppIcon icon="messages-square" size={12} /> Threads
                  </div>
                  <div className="flex items-center gap-2 rounded-[4px] px-2 py-1">
                    <AppIcon icon="activity" size={12} /> Activity
                  </div>
                </div>
                <div className="px-3 pt-2 text-[10px] font-bold text-[#c8b5c9]">
                  Channels
                </div>
                <div className="mt-1 px-2 text-[11px] text-[#d7c8d8]">
                  {channels.map((item) => (
                    <div
                      key={item}
                      className={
                        item === activeChannel
                          ? "flex items-center gap-1.5 rounded-[4px] bg-[#1264a3] px-2 py-1 text-[#f8f8f8]"
                          : "flex items-center gap-1.5 rounded-[4px] px-2 py-1"
                      }
                    >
                      <span className="text-[#bfa9c0]">#</span> {item}
                    </div>
                  ))}
                </div>
                <div className="mt-4 px-3 text-[10px] font-bold text-[#c8b5c9]">
                  Apps
                </div>
                <div className="mt-2 flex items-center gap-2 px-4 text-[11px] text-[#eee7ee]">
                  <HivyAvatar size="small" /> hivy
                </div>
              </aside>
            </>
          ) : null}

          <div className="flex min-w-0 flex-col bg-[#fefefe]">
            <div className="flex h-11 shrink-0 items-center justify-between border-b border-[#dddddd] px-4">
              <div className="min-w-0">
                <p className="truncate text-[13px] font-bold"># {channel}</p>
                <p className="truncate text-[9.5px] text-[#616061]">
                  {channelDescription}
                </p>
              </div>
              <div className="flex items-center gap-3 text-[#616061]">
                <span className="flex items-center gap-1 text-[9px]">
                  <AppIcon icon="users" size={12} /> 24
                </span>
                <AppIcon icon="headset" size={13} />
                <AppIcon icon="ellipsis" size={14} />
              </div>
            </div>
            {children}
          </div>
        </div>
      </motion.div>
    </MotionConfig>
  )
}

export function SlackWorkspaceMockup() {
  return (
    <SlackChrome
      channel="product-support"
      channelDescription="Customer reports, debugging, and handoffs"
    >
      <motion.div
        variants={reveal}
        custom={0.15}
        className="border-b border-[#eeeeee] px-4 py-3"
      >
        <p className="text-[16px] font-bold"># product-support</p>
        <p className="mt-1 text-[10px] text-[#616061]">
          Share support issues and tag hivy when an agent should take over.
        </p>
      </motion.div>
      <div className="py-2">
        <motion.div variants={reveal} custom={0.45}>
          <Message person={people.maya} time="10:42 AM">
            The import fails when the customer has more than one workspace. I
            added the error and a screenshot above. <SlackMention /> can you
            trace this and tell us what changed?
          </Message>
        </motion.div>
        <AnimatedAgentReply
          time="10:43 AM"
          footer={
            <div className="flex flex-wrap gap-1.5">
              <span className="rounded-[4px] border border-[#dddddd] px-2 py-1 text-[9px] text-[#616061]">
                Support agent
              </span>
              <span className="rounded-[4px] border border-[#dddddd] px-2 py-1 text-[9px] text-[#616061]">
                Working in this thread
              </span>
            </div>
          }
        >
          I found the failing path. The workspace lookup now returns a list, but
          the import job still reads one record. I’m checking the last
          deployment and will report back here.
        </AnimatedAgentReply>
      </div>
      <Composer label="Message #product-support" />
    </SlackChrome>
  )
}

export function SlackWatchMockup() {
  return (
    <SlackChrome
      channel="customer-voice"
      activeChannel="customer-voice"
      channelDescription="Customer feedback and product requests"
    >
      <motion.div
        variants={reveal}
        custom={0.15}
        className="flex items-center justify-between border-b border-[#b7d9ea] bg-[#e8f5fa] px-4 py-2.5"
      >
        <div className="flex min-w-0 items-center gap-2.5">
          <HivyAvatar size="small" />
          <div className="min-w-0">
            <p className="truncate text-[11px] font-bold">
              hivy is watching #customer-voice
            </p>
            <p className="mt-0.5 truncate text-[9px] text-[#41697c]">
              Group repeated requests and flag anything that needs a decision.
            </p>
          </div>
        </div>
        <span className="ml-3 flex shrink-0 items-center gap-1.5 rounded-full bg-[#d4edfa] px-2 py-1 text-[9px] font-bold text-[#1264a3]">
          <motion.span
            className="size-1.5 rounded-full bg-[#2eb67d]"
            animate={{ opacity: [0.4, 1, 0.4] }}
            transition={{ duration: 1.8, repeat: Number.POSITIVE_INFINITY }}
          />
          Watching
        </span>
      </motion.div>
      <div className="py-2">
        <motion.div variants={reveal} custom={0.42}>
          <Message person={people.leah} time="9:18 AM">
            Two customers asked for approval history in exported reports this
            morning.
          </Message>
        </motion.div>
        <motion.div variants={reveal} custom={0.85}>
          <Message person={people.omar} time="9:31 AM">
            We heard the same request on yesterday’s onboarding call.
          </Message>
        </motion.div>
        <AnimatedAgentReply
          time="9:32 AM"
          delay={2.18}
          footer={
            <span className="text-[9.5px] font-bold text-[#1264a3]">
              View 2 sources
            </span>
          }
        >
          This request has now appeared in support and onboarding. I added it to
          today’s feedback brief and tagged the product owner for a decision.
        </AnimatedAgentReply>
      </div>
    </SlackChrome>
  )
}

export function SlackReactionMockup() {
  return (
    <SlackChrome
      channel="product-support"
      channelDescription="Customer reports, debugging, and handoffs"
    >
      <motion.div
        variants={reveal}
        custom={0.15}
        className="flex items-center gap-3 border-b border-[#dddddd] bg-[#f8f8f8] px-4 py-2.5"
      >
        <span className="flex size-7 items-center justify-center rounded-[5px] bg-[#fefefe] text-sm shadow-[inset_0_0_0_1px_#dddddd]">
          👀
        </span>
        <div>
          <p className="text-[11px] font-bold">Reaction trigger</p>
          <p className="text-[9px] text-[#616061]">
            When someone adds 👀, run the Support agent on that message.
          </p>
        </div>
      </motion.div>
      <div className="py-2">
        <motion.div variants={reveal} custom={0.46}>
          <Message
            person={people.jon}
            time="2:06 PM"
            footer={
              <div className="flex items-center gap-1.5">
                <motion.span
                  className="rounded-full border border-[#1264a3] bg-[#e8f5fa] px-2 py-0.5 text-[10px] font-bold text-[#1264a3]"
                  initial={{ scale: 0.8 }}
                  whileInView={{ scale: [0.8, 1.12, 1] }}
                  viewport={{ once: true }}
                  transition={{ delay: 0.95, duration: 0.48, ease: easeOut }}
                >
                  👀 2
                </motion.span>
                <span className="rounded-full border border-[#dddddd] px-2 py-0.5 text-[10px] text-[#616061]">
                  +
                </span>
              </div>
            }
          >
            New SSO setup is failing after the domain verification step. Error
            screenshot is attached.
          </Message>
        </motion.div>
        <motion.div
          variants={reveal}
          custom={1.22}
          className="mx-4 my-1.5 flex items-center gap-2 border-y border-[#eeeeee] py-2 text-[9.5px] text-[#616061]"
        >
          <HivyAvatar size="small" /> Reaction matched. hivy started the Support
          agent with this message and its thread.
        </motion.div>
        <AnimatedAgentReply time="2:07 PM" delay={2.45}>
          I picked this up from the 👀 reaction. I’m checking the verification
          callback and will reply in this thread with what failed.
        </AnimatedAgentReply>
      </div>
    </SlackChrome>
  )
}

export function SlackThreadContinuityMockup() {
  return (
    <SlackChrome
      focused
      channel="product-support"
      channelDescription="Customer reports, debugging, and handoffs"
    >
      <div className="grid min-h-[385px] md:grid-cols-[minmax(0,0.92fr)_minmax(330px,1.08fr)]">
        <div className="border-b border-[#dddddd] bg-[#fefefe] py-3 md:border-r md:border-b-0">
          <motion.div variants={reveal} custom={0.2}>
            <Message person={people.maya} time="10:42 AM">
              The import still fails for multi-workspace accounts.{" "}
              <SlackMention /> can you take this?
            </Message>
          </motion.div>
          <motion.div
            variants={reveal}
            custom={0.48}
            className="mt-1 ml-[3.85rem] flex items-center gap-2 text-[10px] font-bold text-[#1264a3]"
          >
            <span className="flex -space-x-1">
              <span
                className="size-5 rounded-[4px] bg-cover bg-center ring-2 ring-[#fefefe]"
                style={{ backgroundImage: `url(${people.maya.image})` }}
              />
              <span className="flex size-5 items-center justify-center rounded-[4px] bg-[#fff7ef] p-1 ring-2 ring-[#fefefe]">
                <LogoMark className="size-full" />
              </span>
            </span>
            4 replies
            <span className="font-normal text-[#616061]">
              Last reply just now
            </span>
          </motion.div>
        </div>

        <motion.div
          variants={{
            hidden: { opacity: 0, x: 18 },
            show: {
              opacity: 1,
              x: 0,
              transition: { delay: 0.55, duration: 0.5, ease: easeOut },
            },
          }}
          className="flex min-w-0 flex-col bg-[#fefefe]"
        >
          <div className="flex h-11 shrink-0 items-center justify-between border-b border-[#dddddd] px-4">
            <div className="flex items-baseline gap-2">
              <span className="text-[13px] font-bold">Thread</span>
              <span className="text-[9px] text-[#616061]">
                #product-support
              </span>
            </div>
            <AppIcon icon="x" size={14} className="text-[#616061]" />
          </div>
          <div className="border-b border-[#eeeeee] py-1.5">
            <Message person={people.maya} time="10:42 AM" compact>
              The import still fails for multi-workspace accounts.{" "}
              <SlackMention /> can you take this?
            </Message>
          </div>
          <div className="flex items-center gap-2 px-4 py-2 text-[9px] text-[#616061] before:h-px before:flex-1 before:bg-[#dddddd] after:h-px after:flex-1 after:bg-[#dddddd]">
            4 replies
          </div>
          <motion.div variants={reveal} custom={1.02}>
            <Message time="10:43 AM" app compact>
              I found the failing lookup and started a fix.
            </Message>
          </motion.div>
          <motion.div variants={reveal} custom={1.45}>
            <Message person={people.maya} time="10:51 AM" compact>
              Does this affect imports that started yesterday?
            </Message>
          </motion.div>
          <AnimatedAgentReply time="10:52 AM" delay={2.65}>
            Yes. I’m checking those jobs in the same session now.
          </AnimatedAgentReply>
          <Composer label="Reply…" />
        </motion.div>
      </div>
    </SlackChrome>
  )
}

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
              <SlackMention /> we’re changing the importer again. What should we
              avoid breaking?
            </Message>
          </motion.div>
          <AnimatedAgentReply time="11:09 AM" delay={1.7}>
            Keep the workspace lookup list-aware. The last failure came from
            treating a multi-workspace account as one record.
          </AnimatedAgentReply>
        </div>
      </motion.div>
    </MotionConfig>
  )
}
