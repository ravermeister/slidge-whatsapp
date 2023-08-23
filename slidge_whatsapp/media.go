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
	"io"
	"math"
	"os"
	"os/exec"
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
	ffmpegDefaultArgs = []string{"-v", "error", "-i", "pipe:0"}
)

// The full path and default arguments for FFprobe, as provided by FFmpeg, used for getting media
// metadata (e.g. duration, waveforms, etc.)
var (
	ffprobeCommand, _  = exec.LookPath("ffprobe")
	ffprobeDefaultArgs = []string{"-v", "error", "-of", "csv=nokey=0:print_section=0"}
)

const (
	// The output specification to use when writing converted media to a pipe.
	convertPipeOutput = "pipe:1"
)

const (
	// The MIME type used by voice messages on WhatsApp.
	voiceMessageMIME = "audio/ogg; codecs=opus"
	// The MIME type used by video messages on WhatsApp.
	videoMessageMIME = "video/mp4"
	// The audio MIME type corresponding to that for video messages.
	videoAudioMIME = "audio/mp4"
	// the MIME type used by animated images on WhatsApp.
	animatedImageMIME = "image/gif"
)

// A ConvertMediaFunc is a function that can convert any data buffer to another, given a set of
// arguments.
type convertMediaFunc func([]byte, ...string) ([]byte, error)

// ConvertMediaOptions contains options used in converting media between formats via FFmpeg.
type convertMediaOptions struct {
	mime string           // The destination MIME type for the converted media.
	call convertMediaFunc // The function to use for converting media.
	args []string         // The arguments to pass to the conversion function.
}

// Media conversion specifications.
var (
	// The MIME type and conversion arguments used by image messages on WhatsApp.
	imageMessageOptions = convertMediaOptions{
		mime: "image/jpeg",
		call: convertImage,
	}
	// The MIME type and conversion arguments used by voice messages on WhatsApp.
	voiceMessageOptions = convertMediaOptions{
		mime: voiceMessageMIME,
		call: convertAudioVideo,
		args: append(
			ffmpegDefaultArgs,
			"-f", "ogg", "-c:a", "libopus", // Convert to Ogg with Opus.
			"-ac", "1", // Convert to mono.
			"-ar", "48000", // Use specific sample-rate of 48000hz.
			"-b:a", "16k", // Use relatively small bit-rate of 16kBit/s.
			"-map_metadata", "-1", // Remove all metadata from output.
			convertPipeOutput, // Write to pipe.
		),
	}
	// The MIME type and conversion arguments used by video messages on WhatsApp.
	videoMessageOptions = convertMediaOptions{
		mime: videoMessageMIME,
		call: convertAudioVideo,
		args: append(
			ffmpegDefaultArgs,
			"-f", "mp4", "-c:v", "libx264", // Convert to mp4 with h264.
			"-pix_fmt", "yuv420p", // Use YUV 4:2:0 chroma subsampling.
			"-profile:v", "baseline", // Use Baseline profile for better compatibility.
			"-level", "3.0", // Ensure compatibility with older devices.
			"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", // Pad dimensions to ensure height is a factor of 2.
			"-r", "25", "-g", "50", // Use 25fps, with an index frame every 50 frames.
			"-c:a", "aac", "-b:a", "160k", "-r:a", "44100", // Re-encode audio to AAC, if any.
			"-movflags", "+faststart", // Use Faststart for quicker rendering.
			"-y", // Overwrite existing output file, where this exists.
		),
	}
)

// ConvertMediaTypes represents a list of media types to convert based on source MIME type.
var convertMediaTypes = map[string]convertMediaOptions{
	"image/png":  imageMessageOptions,
	"image/webp": imageMessageOptions,
	"audio/mp4":  voiceMessageOptions,
	"audio/aac":  voiceMessageOptions,
	"video/mp4":  videoMessageOptions,
	"video/webm": videoMessageOptions,
	"image/gif": {
		mime: videoMessageOptions.mime,
		call: videoMessageOptions.call,
		args: append([]string{
			"-f", "gif_pipe", // Use special GIF encoder for reading from pipe.
			"-r", "10", // Assume 10fps GIF speed.
		}, videoMessageOptions.args...),
	},
}

// ConvertAttachment attempts to process a given attachment from a less-supported type to a
// canonically supported one; for example, from `image/png` to `image.jpeg`. Decisions about which
// MIME types to convert to are based on the origin MIME type, and care is taken to conform to
// WhatsApp semantics for the given input MIME type. If the input MIME type is unknown, or
// conversion is impossible, the original attachment is returned unchanged.
func convertAttachment(attach Attachment) (Attachment, error) {
	if attach.MIME == "" || attach.MIME == "application/octet-stream" {
		if t, _ := filetype.Match(attach.Data); t != filetype.Unknown {
			attach.MIME = t.MIME.Value
		}
	}

	// Try to see if there's a video stream for ostensibly video-related MIME types, as these are
	// some times misdetected as such.
	if attach.MIME == videoMessageMIME {
		if m, err := getMediaMetadata(attach.Data); err == nil {
			attach.meta = m
			if m.width == 0 && m.height == 0 && m.sampleRate > 0 && m.duration > 0 {
				attach.MIME = videoAudioMIME
			}
		}
	}

	if o, ok := convertMediaTypes[attach.MIME]; ok {
		if data, err := o.call(attach.Data, o.args...); err != nil {
			return attach, fmt.Errorf("conversion from %s to %s failed: %s", attach.MIME, o.mime, err)
		} else if len(data) > 0 {
			attach.Data, attach.MIME = data, o.mime
		}
	}

	return attach, nil
}

const (
	// The maximum image buffer size we'll attempt to process in any way, in bytes.
	maxImageSize = 1024 * 1024 * 10 // 10MiB
	// The maximum media buffer size we'll attempt to process in any way, in bytes.
	maxMediaSize = 1024 * 1024 * 20 // 20MiB
)

// ConvertImage returns a buffer containing a JPEG-encoded image, as converted from the source image
// given as data. Any error in conversion will return the error and a nil image.
func convertImage(data []byte, args ...string) ([]byte, error) {
	if len(data) > maxImageSize {
		return nil, fmt.Errorf("buffer size %d exceeds maximum of %d", len(data), maxImageSize)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, img, nil); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ConvertAudioVideo returns a data buffer containing a media file, as converted from the given
// media buffer. Any error in processing or converting the media buffer will return immediately.
func convertAudioVideo(data []byte, args ...string) ([]byte, error) {
	if ffmpegCommand == "" {
		return nil, fmt.Errorf("FFmpeg command not found")
	} else if len(data) > maxMediaSize {
		return nil, fmt.Errorf("buffer size %d exceeds maximum of %d", len(data), maxMediaSize)
	}

	cmd := exec.Command(ffmpegCommand, args...)
	cmd.Stdin = bytes.NewReader(data)

	// Certain output file formats require seekable media, and cannot use a streaming pipe for
	// writing. In those cases, we have to write to a temporary file which is subsequently removed.
	var fileout *os.File
	if args[len(args)-1] != convertPipeOutput {
		if f, err := os.CreateTemp("", "slidge-whatsapp-*"); err != nil {
			return nil, fmt.Errorf("failed creating temporary file for writing: %s", err)
		} else if err := f.Close(); err == nil {
			defer func() { f.Close(); os.Remove(f.Name()) }()
			cmd.Args, fileout = append(cmd.Args, f.Name()), f
		}
	}

	result, err := cmd.Output()
	if err != nil {
		if e := new(exec.ExitError); errors.As(err, &e) {
			return nil, fmt.Errorf("%s: %s", e.Error(), bytes.TrimSpace(e.Stderr))
		}
		return nil, err
	}

	if fileout != nil {
		if fileout, err = os.Open(fileout.Name()); err != nil {
			return nil, fmt.Errorf("failed opening temporary file: %s", err)
		} else if result, err = io.ReadAll(fileout); err != nil {
			return nil, fmt.Errorf("failed reading from temporary file: %s", err)
		}
	}

	return result, nil
}

// GetMediaThumbnail returns a static thumbnail in JPEG format from the given video buffer. If no
// thumbnail could be generated for any reason, this returns nil.
func getMediaThumbnail(data []byte) []byte {
	buf, _ := convertAudioVideo(data, "-f", "mjpeg", "-qscale:v", "5", "-frames:v", "1", convertPipeOutput)
	return buf
}

// MediaMetadata represents secondary information for a given audio/video buffer. This information
// is usually gathered on a best-effort basis, and thus may be missing even for otherwise valid
// media buffers.
type mediaMetadata struct {
	width      int           // The calculated width of the given video buffer; 0 if there's no video stream.
	height     int           // The calculated height of the given video buffer; 0 if there's no video stream.
	sampleRate int           // The calculated sample rate of the given audio buffer; usually not set for video streams.
	duration   time.Duration // The duration of the given audio/video stream.
}

// PrepareMediaProbe sets up FFprobe for execution, returning nil if no such command is available.
func prepareMediaProbe(in io.Reader, args ...string) (*exec.Cmd, error) {
	if ffprobeCommand == "" {
		return nil, fmt.Errorf("FFprobe command not found")
	}
	cmd := exec.Command(ffprobeCommand, append(ffprobeDefaultArgs, args...)...)
	cmd.Stdin = in
	return cmd, nil
}

// GetMediaMetadata calculates and returns secondary information for the given audio/video buffer,
// if any. Metadata is gathered on a best-effort basis, and may be missing -- see the documentation
// for [mediaMetata] for more information.
func getMediaMetadata(data []byte) (mediaMetadata, error) {
	if len(data) > maxMediaSize {
		return mediaMetadata{}, fmt.Errorf("buffer size %d exceeds maximum of %d", len(data), maxMediaSize)
	}

	cmd, err := prepareMediaProbe(
		bytes.NewReader(data),
		"-i", "pipe:0",
		"-show_entries", "stream=width,height,sample_rate:packet=dts_time",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return mediaMetadata{}, fmt.Errorf("failed to set up standard output: %s", err)
	} else if err = cmd.Start(); err != nil {
		return mediaMetadata{}, fmt.Errorf("failed to initialize command: %s", err)
	}

	var meta mediaMetadata
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		for _, f := range strings.Split(scanner.Text(), ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(f), "=")
			switch k {
			case "dts_time":
				if v, err := strconv.ParseFloat(v, 64); err == nil {
					meta.duration = time.Duration(v * float64(time.Second))
				}
			case "duration_time":
				if v, err := strconv.ParseFloat(v, 64); err == nil {
					meta.duration += time.Duration(v * float64(time.Second))
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
		return mediaMetadata{}, fmt.Errorf("failed to wait for command to complete: %s", err)
	} else if err = scanner.Err(); err != nil {
		return mediaMetadata{}, fmt.Errorf("failed scanning command output: %s", err)
	}

	return meta, nil
}

const (
	// The maximum number of samples to return in media waveforms.
	maxWaveformSamples = 64
)

// GetMediaWaveform returns the computed waveform for the media buffer given, as a series of 64
// numbers ranging from 0 to 100. Any errors in computing the waveform will have this function
// return a nil result.
func getMediaWaveform(data []byte, meta mediaMetadata) []byte {
	if len(data) > maxMediaSize {
		return nil
	} else if meta == (mediaMetadata{}) {
		var err error
		if meta, err = getMediaMetadata(data); err != nil {
			return nil
		}
	}

	var samples = make([]byte, 0, maxWaveformSamples)
	var numSamples = int(float64(meta.sampleRate)*meta.duration.Seconds()) / maxWaveformSamples

	// Determine number of waveform to take based on duration and sample-rate of original file.
	// Get waveform with 64 samples, and scale these from a range of 0 to 100.
	cmd, err := prepareMediaProbe(bytes.NewReader(data),
		"-f", "lavfi",
		"-i", "amovie=pipe\\\\:0,asetnsamples="+strconv.Itoa(numSamples)+",astats=metadata=1:reset=1",
		"-show_entries", "frame_tags=lavfi.astats.Overall.Peak_level",
	)

	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf

	if cmd.Run() != nil {
		return nil
	}

	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		_, v, _ := bytes.Cut(scanner.Bytes(), []byte{'='})
		db, err := strconv.ParseFloat(string(bytes.Trim(v, "\n\r")), 64)
		if err == nil {
			samples = append(samples, byte(math.Pow(10, (db/50))*100))
		}
	}

	return samples[:maxWaveformSamples]
}
