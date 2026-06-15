"use client"

import { useState, type FormEvent } from "react"
import { Button, InputOTP, Spinner, Typography } from "@heroui/react"

export function EmailConfirmationForm({
  email,
  isConfirming,
  isResending,
  onConfirm,
  onResend,
  onChangeEmail,
}: {
  email: string
  isConfirming: boolean
  isResending: boolean
  onConfirm: (code: string) => void
  onResend: () => void
  onChangeEmail: () => void
}) {
  const [otpValue, setOtpValue] = useState("")

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    onConfirm(otpValue)
  }

  return (
    <form onSubmit={submit} className="flex flex-col items-center gap-6">
      <div className="text-center">
        <Typography.Paragraph size="sm" color="muted">
          Enter the 6-digit code sent to <span>{email}</span>
        </Typography.Paragraph>
      </div>

      <InputOTP
        maxLength={6}
        value={otpValue}
        onChange={(value) => {
          setOtpValue(value)
          if (value.length === 6) {
            onConfirm(value)
          }
        }}
        isDisabled={isConfirming}
      >
        <InputOTP.Group>
          <InputOTP.Slot index={0} />
          <InputOTP.Slot index={1} />
          <InputOTP.Slot index={2} />
        </InputOTP.Group>
        <InputOTP.Separator />
        <InputOTP.Group>
          <InputOTP.Slot index={3} />
          <InputOTP.Slot index={4} />
          <InputOTP.Slot index={5} />
        </InputOTP.Group>
      </InputOTP>

      <Button
        type="submit"
        size="lg"
        fullWidth
        isPending={isConfirming}
        isDisabled={isConfirming || otpValue.length !== 6}
      >
        {({ isPending }) => (
          <>
            {isPending ? <Spinner color="current" size="sm" /> : null}
            {isPending ? "Confirming..." : "Confirm email"}
          </>
        )}
      </Button>

      <div className="flex items-center gap-4">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onPress={onResend}
          isPending={isResending}
          isDisabled={isResending}
        >
          {({ isPending }) => (
            <>
              {isPending ? <Spinner color="current" size="sm" /> : null}
              {isPending ? "Sending..." : "Resend code"}
            </>
          )}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onPress={() => {
            setOtpValue("")
            onChangeEmail()
          }}
          isDisabled={isConfirming}
        >
          Use a different email
        </Button>
      </div>
    </form>
  )
}
