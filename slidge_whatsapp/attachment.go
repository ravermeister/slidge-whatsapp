package whatsapp

import (
	// Standard library.
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	// Third-party packages.
	"github.com/h2non/filetype"
	_ "golang.org/x/image/webp"
)

// The full path and default arguments for FFmpeg, used for converting media to supported types.
var (
	ffmpegCommand, _  = exec.LookPath("ffmpeg")
	ffmpegDefaultArgs = []string{"-v", "error", "-y"}
)

// The full path and default arguments for FFprobe, as provided by FFmpeg, used for getting media
// metadata (e.g. duration, waveforms, etc.)
var (
	ffprobeCommand, _  = exec.LookPath("ffprobe")
	ffprobeDefaultArgs = []string{"-v", "error", "-of", "csv=nokey=0:print_section=0"}
)

const (
	// The MIME type used by voice messages on WhatsApp.
	voiceMessageMIME = "audio/ogg; codecs=opus"
	// the MIME type used by animated images on WhatsApp.
	animatedImageMIME = "image/gif"
)

// A ConvertAttachmentFunc is a function that can convert any attachment to another format, given a
// set of arguments.
type convertAttachmentFunc func(*Attachment, ...string) error

// ConvertAttachmentOptions contains options used in converting media between formats via FFmpeg.
type convertAttachmentOptions struct {
	mime string                // The destination MIME type for the converted media.
	call convertAttachmentFunc // The function to use for converting media.
	args []string              // The arguments to pass to the conversion function.
}

// Attachment conversion specifications.
var (
	// The MIME type and conversion arguments used by image messages on WhatsApp.
	imageMessageOptions = convertAttachmentOptions{
		mime: "image/jpeg",
		call: convertImage,
	}
	// The MIME type and conversion arguments used by voice messages on WhatsApp.
	voiceMessageOptions = convertAttachmentOptions{
		mime: voiceMessageMIME,
		call: convertAudioVideo,
		args: []string{
			"-f", "ogg", "-c:a", "libopus", // Convert to Ogg with Opus.
			"-ac", "1", // Convert to mono.
			"-ar", "48000", // Use specific sample-rate of 48000hz.
			"-b:a", "64k", // Use relatively reasonable bit-rate of 64kBit/s.
			"-map_metadata", "-1", // Remove all metadata from output.
		},
	}
	// The MIME type and conversion arguments used by video messages on WhatsApp.
	videoMessageOptions = convertAttachmentOptions{
		mime: "video/mp4",
		call: convertAudioVideo,
		args: []string{
			"-f", "mp4", "-c:v", "libx264", // Convert to mp4 with h264.
			"-pix_fmt", "yuv420p", // Use YUV 4:2:0 chroma subsampling.
			"-profile:v", "baseline", // Use Baseline profile for better compatibility.
			"-level", "3.0", // Ensure compatibility with older devices.
			"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", // Pad dimensions to ensure height is a factor of 2.
			"-r", "25", "-g", "50", // Use 25fps, with an index frame every 50 frames.
			"-c:a", "aac", "-b:a", "160k", "-r:a", "44100", // Re-encode audio to AAC, if any.
			"-movflags", "+faststart", // Use Faststart for quicker rendering.
			"-y", // Overwrite existing output file, where this exists.
		},
	}
)

// ConvertAttachmentTypes represents a list of media types to convert based on source MIME type.
var convertAttachmentTypes = map[string]convertAttachmentOptions{
	"image/png":              imageMessageOptions,
	"image/webp":             imageMessageOptions,
	"audio/mp4":              voiceMessageOptions,
	"audio/aac":              voiceMessageOptions,
	"audio/ogg; codecs=opus": voiceMessageOptions,
	"video/mp4":              videoMessageOptions,
	"video/webm":             videoMessageOptions,
	"image/gif": {
		mime: videoMessageOptions.mime,
		call: videoMessageOptions.call,
		args: append([]string{
			"-r", "10", // Assume 10fps GIF speed.
		}, videoMessageOptions.args...),
	},
}

// ConvertAttachment attempts to process a given attachment from a less-supported type to a
// canonically supported one; for example, from `image/png` to `image/jpeg`. Decisions about which
// MIME types to convert to are based on the concrete MIME type inferred from the file itself, and
// care is taken to conform to WhatsApp semantics for the given input MIME type. If the input MIME
// type is unknown, or conversion is impossible, the original attachment is returned unchanged.
func convertAttachment(attach *Attachment) error {
	var detectedMIME string
	if t, _ := filetype.MatchFile(attach.Path); t != filetype.Unknown {
		detectedMIME = t.MIME.Value
		if attach.MIME == "" || attach.MIME == "application/octet-stream" {
			attach.MIME = detectedMIME
		}
	}

	switch detectedMIME {
	case "audio/m4a":
		// MP4 audio files are matched as `audio/m4a` which is not a valid MIME, correct this to
		// `audio/mp4`, which is what WhatsApp requires as well.
		detectedMIME = "audio/mp4"
		fallthrough
	case "audio/mp4", "audio/ogg":
		if err := populateAttachmentMetadata(attach); err == nil {
			switch attach.meta.codec {
			// Don't attempt to process lossless files at all, as it's assumed that the sender
			// wants to retain these characteristics. Since WhatsApp will try (and likely fail)
			// to process this as an audio message anyways, set a unique MIME type.
			case "alac":
				attach.MIME = "application/octet-stream"
				return nil
			case "opus":
				detectedMIME += "; codecs=" + attach.meta.codec
			}
		}
	case "video/mp4":
		// Try to see if there's a video stream for ostensibly video-related MIME types, as these are
		// some times misdetected as such.
		if err := populateAttachmentMetadata(attach); err == nil {
			if attach.meta.width == 0 && attach.meta.height == 0 && attach.meta.sampleRate > 0 && attach.meta.duration > 0 {
				detectedMIME = "audio/mp4"
			}
		}
	}

	// Convert attachment between file-types, if source MIME matches the known list of convertable types.
	if o, ok := convertAttachmentTypes[detectedMIME]; ok {
		if err := o.call(attach, o.args...); err != nil {
			return fmt.Errorf("conversion from %s to %s failed: %s", attach.MIME, o.mime, err)
		} else {
			attach.MIME = o.mime
		}
	}

	return nil
}

const (
	// The maximum image attachment size we'll attempt to process in any way, in bytes.
	maxImageSize = 1024 * 1024 * 10 // 10MiB
	// The maximum audio/video attachment size we'll attempt to process in any way, in bytes.
	maxAudioVideoSize = 1024 * 1024 * 20 // 20MiB
)

// ConvertImage processes the given Attachment, assumed to be an image of a supported format, and
// converting to a JPEG-encoded image in-place. No arguments are processed currently.
func convertImage(attach *Attachment, args ...string) error {
	if stat, err := os.Stat(attach.Path); err != nil {
		return err
	} else if s := stat.Size(); s > maxImageSize {
		return fmt.Errorf("attachment size %d exceeds maximum of %d", s, maxImageSize)
	}

	f, err := os.OpenFile(attach.Path, os.O_RDWR, 0)
	if err != nil {
		return err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		f.Close()
		return err
	}

	f.Close()
	if f, err = os.Create(attach.Path); err != nil {
		return err
	}

	if err = jpeg.Encode(f, img, nil); err != nil {
		return err
	}

	return nil
}

// ConvertAudioVideo processes the given Attachment, assumed to be an audio or video file of a
// supported format, according to the arguments given.
func convertAudioVideo(attach *Attachment, args ...string) error {
	if ffmpegCommand == "" {
		return fmt.Errorf("FFmpeg command not found")
	} else if stat, err := os.Stat(attach.Path); err != nil {
		return err
	} else if s := stat.Size(); s > maxAudioVideoSize {
		return fmt.Errorf("attachment size %d exceeds maximum of %d", s, maxAudioVideoSize)
	}

	tmp, err := os.CreateTemp(path.Dir(attach.Path), path.Base(attach.Path)+".*")
	if err != nil {
		return fmt.Errorf("failed creating temporary file: %w", err)
	}

	args = append(ffmpegDefaultArgs, append([]string{"-i", attach.Path}, append(args, tmp.Name())...)...)
	cmd := exec.Command(ffmpegCommand, args...)
	tmp.Close()

	if _, err := cmd.Output(); err != nil {
		if e := new(exec.ExitError); errors.As(err, &e) {
			return fmt.Errorf("%s: %s", e.Error(), bytes.TrimSpace(e.Stderr))
		}
		return err
	}

	if err := os.Rename(tmp.Name(), attach.Path); err != nil {
		return fmt.Errorf("failed cleaning up temporary file: %w", err)
	}

	return nil
}

// GetAttachmentThumbnail returns a static thumbnail in JPEG format from the given Attachment, assumed
// to point to a video file. If no thumbnail could be generated for any reason, this returns nil.
func getAttachmentThumbnail(attach *Attachment) ([]byte, error) {
	var tmp string
	if data, err := os.ReadFile(attach.Path); err != nil {
		return nil, fmt.Errorf("failed reading attachment %s: %w", attach.Path, err)
	} else if tmp, err = createTempFile(data); err != nil {
		return nil, err
	}

	defer os.Remove(tmp)
	var buf []byte

	args := []string{"-f", "mjpeg", "-vf", "scale=500:-1", "-qscale:v", "5", "-frames:v", "1"}
	if err := convertAudioVideo(&Attachment{Path: tmp}, args...); err != nil {
		return nil, err
	} else if buf, err = os.ReadFile(tmp); err != nil {
		return nil, fmt.Errorf("failed reading converted file: %w", err)
	}

	return buf, nil
}

// AttachmentMetadata represents secondary information for a given audio/video buffer. This information
// is usually gathered on a best-effort basis, and thus may be missing even for otherwise valid
// media buffers.
type attachmentMetadata struct {
	codec      string        // The codec used for the primary stream in this attachment.
	width      int           // The calculated width of the given video buffer; 0 if there's no video stream.
	height     int           // The calculated height of the given video buffer; 0 if there's no video stream.
	sampleRate int           // The calculated sample rate of the given audio buffer; usually not set for video streams.
	duration   time.Duration // The duration of the given audio/video stream.
}

// PopulateAttachmentMetadata calculates and populates secondary information for the given
// audio/video attachment, if any. Metadata is gathered on a best-effort basis, and may be missing;
// see the documentation for [attachmentMetadata] for more information.
func populateAttachmentMetadata(attach *Attachment) error {
	if ffprobeCommand == "" {
		return fmt.Errorf("FFprobe command not found")
	} else if stat, err := os.Stat(attach.Path); err != nil {
		return err
	} else if s := stat.Size(); s > maxAudioVideoSize {
		return fmt.Errorf("attachment size %d exceeds maximum of %d", s, maxAudioVideoSize)
	}

	args := append(ffprobeDefaultArgs, []string{
		"-i", attach.Path,
		"-show_entries", "stream=codec_name,width,height,sample_rate,duration",
	}...)

	cmd := exec.Command(ffprobeCommand, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to set up standard output: %s", err)
	} else if err = cmd.Start(); err != nil {
		return fmt.Errorf("failed to initialize command: %s", err)
	}

	var meta attachmentMetadata
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		for _, f := range strings.Split(scanner.Text(), ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(f), "=")
			switch k {
			case "codec_name":
				meta.codec = v
			case "duration":
				if v, err := strconv.ParseFloat(v, 64); err == nil {
					meta.duration = time.Duration(v * float64(time.Second))
				}
			case "width":
				if v, err := strconv.Atoi(v); err == nil {
					meta.width = v
				}
			case "height":
				if v, err := strconv.Atoi(v); err == nil {
					meta.height = v
				}
			case "sample_rate":
				if v, err := strconv.Atoi(v); err == nil {
					meta.sampleRate = v
				}
			}
		}
	}

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait for command to complete: %s", err)
	} else if err = scanner.Err(); err != nil {
		return fmt.Errorf("failed scanning command output: %s", err)
	}

	attach.meta = meta
	return nil
}

const (
	// The maximum number of samples to return in media waveforms.
	maxWaveformSamples = 64
)

// GetAttachmentWaveform returns the computed waveform for the attachment given, as a series of 64
// numbers ranging from 0 to 100. Any errors in computing the waveform will have this function
// return a nil result.
func getAttachmentWaveform(attach *Attachment) ([]byte, error) {
	if ffprobeCommand == "" {
		return nil, fmt.Errorf("FFprobe command not found")
	} else if stat, err := os.Stat(attach.Path); err != nil {
		return nil, err
	} else if s := stat.Size(); s > maxAudioVideoSize {
		return nil, fmt.Errorf("attachment size %d exceeds maximum of %d", s, maxAudioVideoSize)
	} else if attach.meta.sampleRate == 0 || attach.meta.duration == 0 {
		return nil, fmt.Errorf("empty sample-rate or duration")
	}

	var samples = make([]byte, 0, maxWaveformSamples)
	var numSamples = int(float64(attach.meta.sampleRate)*attach.meta.duration.Seconds()) / maxWaveformSamples

	// Determine number of waveform to take based on duration and sample-rate of original file.
	// Get waveform with 64 samples, and scale these from a range of 0 to 100.
	args := append(ffprobeDefaultArgs, []string{
		"-f", "lavfi",
		"-i", "amovie=" + attach.Path + ",asetnsamples=" + strconv.Itoa(numSamples) + ",astats=metadata=1:reset=1",
		"-show_entries", "frame_tags=lavfi.astats.Overall.Peak_level",
	}...)

	var buf bytes.Buffer
	cmd := exec.Command(ffprobeCommand, args...)
	cmd.Stdout = &buf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run command: %w", err)
	}

	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		_, v, _ := bytes.Cut(scanner.Bytes(), []byte{'='})
		db, err := strconv.ParseFloat(string(bytes.Trim(v, "\n\r")), 64)
		if err == nil {
			samples = append(samples, byte(math.Pow(10, (db/50))*100))
		}
	}

	return samples[:maxWaveformSamples], nil
}
