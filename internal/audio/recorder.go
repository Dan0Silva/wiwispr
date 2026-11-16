package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	SampleRate    = 16000
	Channels      = 1
	BytesPerFrame = 2 // 16-bit signed little-endian (s16le)
	ChunkDuration = 50 * time.Millisecond
	// 16000 * 0.050 * 2 bytes = 1600 bytes per 50ms chunk (800 samples)
	ChunkBytes = (SampleRate * BytesPerFrame) / 20
)

// StopReason describes why recording finished.
type StopReason string

const (
	StopReasonSilence        StopReason = "silence_detected"
	StopReasonManualToggle   StopReason = "manual_toggle"
	StopReasonInitialTimeout StopReason = "no_speech_timeout"
	StopReasonMaxDuration    StopReason = "max_duration_reached"
)

// RecordResult holds the captured audio metadata.
type RecordResult struct {
	WAVPath       string
	Duration      time.Duration
	SpeechDetected bool
	Reason        StopReason
}

// LevelCallback receives the normalized audio level (0.0 to 1.0) and whether speech has started.
type LevelCallback func(normLevel float64, speaking bool)

// CalculateRMS computes the normalized Root Mean Square (0.0 to 1.0) of 16-bit signed LE PCM data.
func CalculateRMS(pcm []byte) float64 {
	numSamples := len(pcm) / 2
	if numSamples == 0 {
		return 0
	}
	var sumSquares float64
	for i := 0; i < numSamples*2; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		norm := float64(sample) / 32768.0
		sumSquares += norm * norm
	}
	return math.Sqrt(sumSquares / float64(numSamples))
}

// VADTracker implements stateful Voice Activity Detection over 50ms PCM chunks.
type VADTracker struct {
	Threshold         float64
	SilenceLimit      time.Duration
	InitialWaitLimit  time.Duration
	MaxDurationLimit  time.Duration
	MinSpeechDuration time.Duration

	speechStarted   bool
	speechDuration  time.Duration
	silenceDuration time.Duration
	totalDuration   time.Duration
	noiseFloor      float64
	calibratedChunks int
}

// NewVADTracker creates a configured Voice Activity Detector.
func NewVADTracker(threshold float64, silenceLimit, initialWait, maxDuration time.Duration) *VADTracker {
	return &VADTracker{
		Threshold:         threshold,
		SilenceLimit:      silenceLimit,
		InitialWaitLimit:  initialWait,
		MaxDurationLimit:  maxDuration,
		MinSpeechDuration: 150 * time.Millisecond,
	}
}

// ProcessChunk evaluates a single PCM chunk of duration d and returns (shouldStop, reason, visualLevel).
func (v *VADTracker) ProcessChunk(pcm []byte, d time.Duration) (bool, StopReason, float64) {
	rms := CalculateRMS(pcm)
	v.totalDuration += d

	// Adaptive noise floor calibration during the first 250ms if quiet
	if v.calibratedChunks < 5 {
		v.calibratedChunks++
		if rms < v.Threshold {
			v.noiseFloor = (v.noiseFloor*float64(v.calibratedChunks-1) + rms) / float64(v.calibratedChunks)
		}
	}

	effectiveThreshold := v.Threshold
	if v.noiseFloor*2.2 > effectiveThreshold && v.noiseFloor*2.2 < 0.08 {
		effectiveThreshold = v.noiseFloor * 2.2
	}

	isVoice := rms >= effectiveThreshold

	if isVoice {
		v.speechDuration += d
		v.silenceDuration = 0
		if v.speechDuration >= v.MinSpeechDuration {
			v.speechStarted = true
		}
	} else {
		v.silenceDuration += d
	}

	// Calculate visual level (0.0 to 1.0) with logarithmic/curve boost so normal speech looks lively
	visual := 0.0
	if rms > v.noiseFloor {
		adj := (rms - v.noiseFloor) * 9.5
		visual = math.Min(1.0, math.Pow(adj, 0.75))
	}

	if v.speechStarted && v.silenceDuration >= v.SilenceLimit {
		return true, StopReasonSilence, visual
	}
	if !v.speechStarted && v.totalDuration >= v.InitialWaitLimit {
		return true, StopReasonInitialTimeout, visual
	}
	if v.totalDuration >= v.MaxDurationLimit {
		return true, StopReasonMaxDuration, visual
	}

	return false, "", visual
}

// SpeechDetected reports whether meaningful speech was heard during the session.
func (v *VADTracker) SpeechDetected() bool {
	return v.speechStarted
}

// RecordStream captures 16kHz mono s16le audio from PipeWire (pw-record) or ALSA (arecord),
// feeds levels to onLevel, stops automatically on silence or when stopCh/ctx triggers,
// and writes a valid RIFF WAV file to wavOutPath.
func RecordStream(
	ctx context.Context,
	wavOutPath string,
	threshold float64,
	silenceLimit time.Duration,
	initialWait time.Duration,
	maxDuration time.Duration,
	stopCh <-chan struct{},
	onLevel LevelCallback,
) (*RecordResult, error) {
	cmd, err := buildRecordCommand(ctx)
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start audio recorder: %w", err)
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(300 * time.Millisecond):
				_ = cmd.Process.Kill()
			}
		}
	}()

	type chunkMsg struct {
		data []byte
		err  error
	}
	chunkCh := make(chan chunkMsg, 8)

	go func() {
		defer close(chunkCh)
		for {
			buf := make([]byte, ChunkBytes)
			n, err := io.ReadFull(stdout, buf)
			if n > 0 {
				chunkCh <- chunkMsg{data: buf[:n], err: err}
			}
			if err != nil {
				if n == 0 {
					chunkCh <- chunkMsg{err: err}
				}
				return
			}
		}
	}()

	vad := NewVADTracker(threshold, silenceLimit, initialWait, maxDuration)
	var pcmBuffer bytes.Buffer
	// Pre-allocate ~15 seconds of PCM buffer (480 KB)
	pcmBuffer.Grow(SampleRate * BytesPerFrame * 15)

	var finalReason StopReason

loop:
	for {
		select {
		case <-ctx.Done():
			finalReason = StopReasonManualToggle
			break loop
		case <-stopCh:
			finalReason = StopReasonManualToggle
			break loop
		case msg, ok := <-chunkCh:
			if !ok {
				finalReason = StopReasonManualToggle
				break loop
			}
			if len(msg.data) > 0 {
				pcmBuffer.Write(msg.data)
				shouldStop, reason, visLevel := vad.ProcessChunk(msg.data, ChunkDuration)
				if onLevel != nil {
					onLevel(visLevel, vad.SpeechDetected())
				}
				if shouldStop {
					finalReason = reason
					break loop
				}
			}
			if msg.err != nil {
				if errors.Is(msg.err, io.EOF) || errors.Is(msg.err, io.ErrUnexpectedEOF) {
					finalReason = StopReasonManualToggle
					break loop
				}
				return nil, fmt.Errorf("read audio stream: %w", msg.err)
			}
		}
	}

	rawPCM := pcmBuffer.Bytes()
	duration := time.Duration(float64(len(rawPCM))/float64(SampleRate*BytesPerFrame)*float64(time.Second))

	if len(rawPCM) == 0 || (!vad.SpeechDetected() && finalReason == StopReasonInitialTimeout) {
		return &RecordResult{
			WAVPath:        wavOutPath,
			Duration:       duration,
			SpeechDetected: false,
			Reason:         finalReason,
		}, nil
	}

	if err := WriteWAVFile(wavOutPath, rawPCM, SampleRate, Channels); err != nil {
		return nil, fmt.Errorf("write wav file: %w", err)
	}

	return &RecordResult{
		WAVPath:        wavOutPath,
		Duration:       duration,
		SpeechDetected: vad.SpeechDetected() || finalReason == StopReasonManualToggle,
		Reason:         finalReason,
	}, nil
}

func buildRecordCommand(ctx context.Context) (*exec.Cmd, error) {
	// Prefer ALSA 'arecord -q -D default -t raw -f S16_LE -r 16000 -c 1' or PipeWire 'pw-cat --record'
	// Note: pw-record outputs WAV header when filename is '-', whereas pw-cat / arecord -t raw outputs raw PCM!
	if path, err := exec.LookPath("arecord"); err == nil {
		return exec.CommandContext(ctx, path,
			"-q",
			"-D", "default",
			"-t", "raw",
			"-f", "S16_LE",
			"-r", "16000",
			"-c", "1",
		), nil
	}
	if path, err := exec.LookPath("pw-record"); err == nil {
		return exec.CommandContext(ctx, path,
			"--format=s16",
			"--rate=16000",
			"--channels=1",
			"-",
		), nil
	}
	return nil, errors.New("neither arecord nor pw-record found in PATH")
}

// WriteWAVFile writes a standard 44-byte RIFF WAVE header followed by raw 16-bit PCM samples.
func WriteWAVFile(path string, pcm []byte, sampleRate, channels int) error {
	// If the source already started with a RIFF header (e.g. from pw-record), strip the first 44 bytes
	if len(pcm) >= 44 && string(pcm[0:4]) == "RIFF" && string(pcm[8:12]) == "WAVE" {
		pcm = pcm[44:]
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	dataSize := uint32(len(pcm))
	byteRate := uint32(sampleRate * channels * BytesPerFrame)
	blockAlign := uint16(channels * BytesPerFrame)

	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)  // AudioFormat 1 = PCM
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], 16) // BitsPerSample
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	if _, err := f.Write(header); err != nil {
		return err
	}
	_, err = f.Write(pcm)
	return err
}
