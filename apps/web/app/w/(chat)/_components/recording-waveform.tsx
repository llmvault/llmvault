"use client"

import { useEffect, useRef } from "react"

const BAR_WIDTH = 2
const BAR_GAP = 4
const MIN_BAR_HEIGHT = 2
const BASELINE_HEIGHT = 1
const LEVEL_GAIN = 2.7
const LEVEL_CURVE = 0.62
const LEVEL_FLOOR = 0.08
const HEIGHT_SCALE = 0.95
const SAMPLE_INTERVAL_MS = 82

export function RecordingWaveform({
  active,
  audioData,
}: {
  active: boolean
  audioData: Uint8Array
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const levelsRef = useRef<number[]>([])
  const lastSampleAtRef = useRef(0)
  const pendingLevelRef = useRef(0)
  const sizeRef = useRef({ width: 0, height: 0, ratio: 1 })

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const resize = () => {
      const ratio = window.devicePixelRatio || 1
      const rect = canvas.getBoundingClientRect()
      const width = Math.max(1, Math.floor(rect.width))
      const height = Math.max(1, Math.floor(rect.height))
      sizeRef.current = { width, height, ratio }
      canvas.width = Math.floor(width * ratio)
      canvas.height = Math.floor(height * ratio)
      drawWaveform(canvas, levelsRef.current, sizeRef.current)
    }

    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    if (!active) {
      levelsRef.current = []
      lastSampleAtRef.current = 0
      pendingLevelRef.current = 0
      drawWaveform(canvas, levelsRef.current, sizeRef.current)
      return
    }

    const now = performance.now()
    pendingLevelRef.current = Math.max(
      pendingLevelRef.current,
      levelFromAudioData(audioData)
    )
    if (
      lastSampleAtRef.current &&
      now - lastSampleAtRef.current < SAMPLE_INTERVAL_MS
    ) {
      return
    }

    levelsRef.current.push(pendingLevelRef.current)
    pendingLevelRef.current = 0
    lastSampleAtRef.current = now
    const maxLevels = Math.floor(sizeRef.current.width / (BAR_WIDTH + BAR_GAP))
    if (maxLevels > 0 && levelsRef.current.length > maxLevels) {
      levelsRef.current = levelsRef.current.slice(-maxLevels)
    }
    drawWaveform(canvas, levelsRef.current, sizeRef.current)
  }, [active, audioData])

  return (
    <canvas
      ref={canvasRef}
      aria-hidden="true"
      className="h-full w-full"
      style={{ color: "var(--foreground)", borderColor: "var(--separator)" }}
    >
      Your browser does not support HTML5 Canvas.
    </canvas>
  )
}

function levelFromAudioData(audioData: Uint8Array) {
  if (!audioData.length) return 0.04

  let sum = 0
  for (const sample of audioData) {
    const centered = (sample - 128) / 128
    sum += centered * centered
  }
  const rms = Math.sqrt(sum / audioData.length)
  return Math.min(
    1,
    Math.max(LEVEL_FLOOR, Math.pow(rms, LEVEL_CURVE) * LEVEL_GAIN)
  )
}

function drawWaveform(
  canvas: HTMLCanvasElement,
  levels: number[],
  size: { width: number; height: number; ratio: number }
) {
  const context = canvas.getContext("2d")
  if (!context) return

  const { width, height, ratio } = size
  context.save()
  context.scale(ratio, ratio)
  context.clearRect(0, 0, width, height)

  const styles = getComputedStyle(canvas)
  const foreground = styles.color
  const quiet = styles.borderTopColor
  const centerY = height / 2

  context.fillStyle = quiet
  context.fillRect(0, centerY - BASELINE_HEIGHT / 2, width, BASELINE_HEIGHT)

  context.fillStyle = foreground
  levels.forEach((level, index) => {
    const distanceFromEnd = levels.length - 1 - index
    const x = width - BAR_WIDTH - distanceFromEnd * (BAR_WIDTH + BAR_GAP)
    if (x < 0) return
    const barHeight = Math.max(MIN_BAR_HEIGHT, level * height * HEIGHT_SCALE)
    context.beginPath()
    context.roundRect(
      x,
      centerY - barHeight / 2,
      BAR_WIDTH,
      barHeight,
      BAR_WIDTH
    )
    context.fill()
  })
  context.restore()
}
