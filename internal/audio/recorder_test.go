package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTonePCM(rmsTarget float64, numSamples int) []byte {
	buf := make([]byte, numSamples*2)
	// For a sine wave, peak = rms * sqrt(2)
	amp := math.Min(32767.0, rmsTarget*math.Sqrt2*32768.0)
	for i := 0; i < numSamples; i++ {
		val := int16(amp * math.Sin(2.0*math.Pi*440.0*float64(i)/float64(SampleRate)))
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], uint16(val))
	}
	return buf
}

func TestCalculateRMS(t *testing.T) {
	silent := make([]byte, 1600)
	if rms := CalculateRMS(silent); rms != 0 {
		t.Fatalf("expected 0 RMS for silent buffer, got %f", rms)
	}

	tone := generateTonePCM(0.10, 800)
	rms := CalculateRMS(tone)
	if math.Abs(rms-0.10) > 0.01 {
		t.Fatalf("expected RMS ~0.10, got %f", rms)
	}
}

func TestVADTracker_AutoStopOnSilenceAfterSpeech(t *testing.T) {
	vad := NewVADTracker(0.02, 2*time.Second, 6*time.Second, 60*time.Second)

	quietChunk := generateTonePCM(0.002, 800) // 50ms of quiet room noise
	voiceChunk := generateTonePCM(0.08, 800)  // 50ms of clear speech

	// 1. 500ms of initial quiet (10 chunks) -> should not stop
	for i := 0; i < 10; i++ {
		stop, _, _ := vad.ProcessChunk(quietChunk, 50*time.Millisecond)
		if stop {
			t.Fatalf("unexpected stop during initial quiet at chunk %d", i)
		}
	}

	// 2. 1 second of speech (20 chunks) -> speechStarted must become true
	for i := 0; i < 20; i++ {
		stop, _, _ := vad.ProcessChunk(voiceChunk, 50*time.Millisecond)
		if stop {
			t.Fatalf("unexpected stop during active speech at chunk %d", i)
		}
	}
	if !vad.SpeechDetected() {
		t.Fatal("expected SpeechDetected() to be true after 1s of voice")
	}

	// 3. 1.95 seconds of silence (39 chunks) -> should NOT stop yet
	for i := 0; i < 39; i++ {
		stop, _, _ := vad.ProcessChunk(quietChunk, 50*time.Millisecond)
		if stop {
			t.Fatalf("stopped too early at silence chunk %d", i)
		}
	}

	// 4. 40th chunk (2.00s of silence) -> MUST trigger StopReasonSilence
	stop, reason, _ := vad.ProcessChunk(quietChunk, 50*time.Millisecond)
	if !stop || reason != StopReasonSilence {
		t.Fatalf("expected stop=true, reason=StopReasonSilence at 2.0s; got stop=%v, reason=%s", stop, reason)
	}
}

func TestWriteWAVFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.wav")
	pcm := generateTonePCM(0.05, 1600)
	if err := WriteWAVFile(tmp, pcm, SampleRate, Channels); err != nil {
		t.Fatalf("WriteWAVFile failed: %v", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(data) != 44+len(pcm) {
		t.Fatalf("expected file size %d, got %d", 44+len(pcm), len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header")
	}
}
