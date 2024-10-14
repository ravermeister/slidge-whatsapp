package media

import (
	// Standard library.
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	// Third-party packages.
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MIMEType represents a the media type for a data buffer. In general, values given concrete [MIMEType]
// identities are meant to be handled as targets for conversion and metadata extraction -- all other
// formats are handled on a best-case basis.
type MIMEType string

// BaseMediaType returns the media type without any additional parameters.
func (t MIMEType) BaseMediaType() MIMEType {
	return MIMEType(strings.SplitN(string(t), ";", 2)[0])
}

const (
	// Audio formats.
	TypeM4A MIMEType = "audio/mp4"
	TypeOgg MIMEType = "audio/ogg"

	// Video formats.
	TypeMP4 MIMEType = "video/mp4"

	// Image formats.
	TypeJPEG MIMEType = "image/jpeg"
	TypePNG  MIMEType = "image/png"
)

// AudioCodec represents the encoding method used for an audio stream.
type AudioCodec string

// VideoCodec represents the encoding method used for a video stream.
type VideoCodec string

const (
	// Audio codecs.
	CodecOpus AudioCodec = "opus"
	CodecAAC  AudioCodec = "aac"

	// Video codecs.
	CodecH264 VideoCodec = "h264"
)

// Error messages.
const (
	errInvalidCodec = "media with MIME type %s only support codec %s currently, invalid codec %s chosen"
)

// Spec represents the description of a target media file; depending on platform support and media
// conversion intricacies, source media files can be of any type, even types that aren't represented
// here. Nevertheless, it is intended that media types and codecs represented here are valid as both
// input and output formats.
type Spec struct {
	// Required parameters.
	MIME MIMEType // The MIME type for the target media.

	// Optional parameters.
	AudioCodec      AudioCodec // The codec to use for the audio stream, must be correct for the MIME type given.
	AudioChannels   int        // The number of channels for the audio stream, 1 for mono, 2 for stereo.
	AudioBitRate    int        // The bit rate for the audio stream, in kBit/second.
	AudioSampleRate int        // The sample-rate frequency for the audio stream, common values are 44100, 48000.

	VideoCodec       VideoCodec // The codec to use for the video stream, must be correct for the MIME type given.
	VideoPixelFormat string     // The pixel format used for video stream, typically 'yub420p' for MP4.
	VideoFrameRate   int        // The frame rate for the video stream, in frames/second.
	VideoWidth       int        // The width of the video stream, in pixels.
	VideoHeight      int        // The height of the video stream, in pixels.
	VideoFilter      string     // A complex filter to apply to the video stream.

	ImageWidth     int // The width of the image, in pixels.
	ImageHeight    int // The height of the image, in pixels.
	ImageQuality   int // Image quality for lossy image formats, typically a value from 1 to 100.
	ImageFrameRate int // The frame-rate for animated images.

	Duration      time.Duration // The duration of the audio or video stream.
	StripMetadata bool          // Whether or not to remove any container-level metadata present in the stream.
}

// CommandLineArgs returns the current [Spec] as a list of command-line arguments meant for FFMPEG
// invocations. Where the specification is missing values, default values will be filled where
// necessary; however, invalid values may have this function return errors.
func (s Spec) commandLineArgs() ([]string, error) {
	var args []string
	var mime = s.MIME.BaseMediaType()

	switch mime {
	case TypeOgg, TypeM4A:
		// Audio file format parameters.
		switch mime {
		case TypeOgg:
			if s.AudioCodec != "" && s.AudioCodec != CodecOpus {
				return nil, fmt.Errorf(errInvalidCodec, mime, CodecOpus, s.AudioCodec)
			}
			args = append(args, "-f", "ogg", "-c:a", "libopus")
		case TypeM4A:
			if s.AudioCodec != "" && s.AudioCodec != CodecAAC {
				return nil, fmt.Errorf(errInvalidCodec, mime, CodecAAC, s.AudioCodec)
			}
			args = append(args, "-f", "ipod", "-c:a", "aac")
		}

		if s.AudioChannels > 0 {
			args = append(args, "-ac", strconv.Itoa(s.AudioChannels))
		}
		if s.AudioBitRate > 0 {
			args = append(args, "-b:a", strconv.Itoa(s.AudioBitRate)+"k")
		}
		if s.AudioSampleRate > 0 {
			args = append(args, "-ar", strconv.Itoa(s.AudioSampleRate))
		}
	case TypeMP4:
		// Video file format parameters.
		if s.VideoCodec != "" && s.VideoCodec != CodecH264 {
			return nil, fmt.Errorf(errInvalidCodec, mime, CodecH264, s.VideoCodec)
		} else if s.AudioCodec != "" && s.AudioCodec != CodecAAC {
			return nil, fmt.Errorf(errInvalidCodec, mime, CodecAAC, s.AudioCodec)
		}

		// Set input image frame-rate, e.g. when converting from GIF to MP4.
		if s.ImageFrameRate > 0 {
			args = append(args, "-r", strconv.Itoa(s.ImageFrameRate))
		}

		args = append(args,
			"-f", "mp4", "-c:v", "libx264", "-c:a", "aac",
			"-profile:v", "baseline", // Use Baseline profile for better compatibility.
			"-level", "3.0", // Ensure compatibility with older devices.
			"-movflags", "+faststart", // Use Faststart for quicker rendering.
		)

		if s.VideoPixelFormat != "" {
			args = append(args, "-pix_fmt", s.VideoPixelFormat)
		}
		if s.VideoFilter != "" {
			args = append(args, "-filter:v", s.VideoFilter)
		}
		if s.VideoFrameRate > 0 {
			args = append(args,
				"-r", strconv.Itoa(s.VideoFrameRate),
				"-g", strconv.Itoa(s.VideoFrameRate*2),
			)
		}
		if s.AudioBitRate > 0 {
			args = append(args, "-b:a", strconv.Itoa(s.AudioBitRate)+"k")
		}
		if s.AudioSampleRate > 0 {
			args = append(args, "-r:a", strconv.Itoa(s.AudioSampleRate))
		}
	case TypeJPEG, TypePNG:
		// Simple image formats process [Spec] parameters directly, and need no further processing.
		return []string{}, nil
	default:
		return nil, fmt.Errorf("cannot process media specification for empty or unknown MIME type")
	}

	if s.StripMetadata {
		args = append(args, "-map_metadata", "-1")
	}

	return args, nil
}

// Convert processes the given data, assumed to represent a media file, according to the target
// specification given. For information on how these definitions affect media conversions, see the
// documentation for the [Spec] type.
func Convert(ctx context.Context, data []byte, spec *Spec) ([]byte, error) {
	switch spec.MIME.BaseMediaType() {
	case TypeOgg, TypeM4A, TypeMP4:
		return convertAudioVideo(ctx, data, spec)
	case TypeJPEG, TypePNG:
		return convertImage(ctx, data, spec)
	default:
		return nil, fmt.Errorf("unknown media type given in specification")
	}
}

// ConvertAudioVideo processes the given audio/video data via FFmpeg, for the target specification
// given. Calls to FFmpeg will be given arguments as per [Spec.commandLineArgs].
func convertAudioVideo(ctx context.Context, data []byte, spec *Spec) ([]byte, error) {
	args, err := spec.commandLineArgs()
	if err != nil {
		return nil, err
	}

	in, err := createTempFile(data)
	if err != nil {
		return nil, err
	}

	defer os.Remove(in)

	out, err := createTempFile(nil)
	if err != nil {
		return nil, err
	}

	defer os.Remove(out)

	if err := ffmpeg(ctx, in, out, args...); err != nil {
		return nil, err
	}

	return os.ReadFile(out)
}

// ConvertImage processes the following image data given via Go-native image processing. Currently,
// only JPEG and PNG output is allowed, as set in the [Spec.MIME] field.
func convertImage(_ context.Context, data []byte, spec *Spec) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	// Resize image if dimensions given in spec, retaining aspect ratio if either width or height
	// aren't provided.
	if spec.ImageWidth > 0 || spec.ImageHeight > 0 {
		width, height := spec.ImageWidth, spec.ImageHeight
		if width == 0 {
			width = int(float64(img.Bounds().Max.X) / (float64(img.Bounds().Max.Y) / float64(height)))
		} else if height == 0 {
			height = int(float64(img.Bounds().Max.Y) / (float64(img.Bounds().Max.X) / float64(width)))
		}

		tmp := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.ApproxBiLinear.Scale(tmp, tmp.Rect, img, img.Bounds(), draw.Over, nil)
		img = tmp
	}

	// Re-encode image based on target MIME type.
	var out bytes.Buffer
	switch spec.MIME.BaseMediaType() {
	case TypeJPEG:
		o := jpeg.Options{Quality: spec.ImageQuality}
		if o.Quality == 0 {
			o.Quality = jpeg.DefaultQuality
		}

		if err = jpeg.Encode(&out, img, nil); err != nil {
			return nil, err
		}
	case TypePNG:
		if err = png.Encode(&out, img); err != nil {
			return nil, err
		}
	}

	return out.Bytes(), nil
}

// GetSpec returns a media specification corresponding to the data given. The [Spec] value returned
// will only have its fields partially populated, as not all values can be derived accurately.
func GetSpec(ctx context.Context, data []byte) (*Spec, error) {
	in, err := createTempFile(data)
	if err != nil {
		return nil, err
	}

	defer os.Remove(in)

	out, err := ffprobe(ctx, in,
		"-show_entries", "stream=codec_name,width,height,sample_rate,duration",
	)

	var result Spec
	if s, ok := out["streams"].([]any); ok {
		if len(s) == 0 {
			return nil, fmt.Errorf("no valid audio/video streams found in data")
		} else if r, ok := s[0].(map[string]any); ok {
			if v, ok := r["duration"].(string); ok {
				if v, err := strconv.ParseFloat(v, 64); err == nil {
					result.Duration = time.Duration(v * float64(time.Second))
				}
			}
			if v, ok := r["width"].(string); ok {
				if v, err := strconv.Atoi(v); err == nil {
					result.VideoWidth = v
				}
			}
			if v, ok := r["height"].(string); ok {
				if v, err := strconv.Atoi(v); err == nil {
					result.VideoHeight = v
				}
			}
			if v, ok := r["sample_rate"].(string); ok {
				if v, err := strconv.Atoi(v); err == nil {
					result.AudioSampleRate = v
				}
			}
			if v, ok := r["codec_name"].(string); ok {
				if result.VideoWidth > 0 || result.VideoHeight > 0 {
					result.VideoCodec = VideoCodec(v)
				} else {
					result.AudioCodec = AudioCodec(v)
				}
			}
		}
	}

	return &result, nil
}

// GetThumbnail returns a static JPEG image for the first frame of the given video data. If both
// width and height dimensions are given, the thumbnail will be resized to exactly those pixel
// values. If either dimension is given, the thumbnail will be resized while retaining aspect ratio.
// If no value is given (i.e. if both values are zero), then the thumbnail dimensions will follow
// input video dimensions.
func GetThumbnail(ctx context.Context, data []byte, width, height int) ([]byte, error) {
	in, err := createTempFile(data)
	if err != nil {
		return nil, err
	}

	defer os.Remove(in)

	out, err := createTempFile(nil)
	if err != nil {
		return nil, err
	}

	defer os.Remove(out)

	args := []string{
		"-f", "mjpeg",
		"-qscale:v", "5",
		"-frames:v", "1",
		"-map_metadata", "-1",
	}

	// Scale thumbnail if width/height pixel factors given.
	if width > 0 || height > 0 {
		if width == 0 {
			width = -1
		} else if height == 0 {
			height = -1
		}
		args = append(args, "-vf", "scale="+strconv.FormatInt(int64(width), 10)+":"+strconv.FormatInt(int64(height), 10))
	}

	if err := ffmpeg(ctx, in, out, args...); err != nil {
		return nil, err
	}

	return os.ReadFile(out)
}

// GetWaveform returns a list of samples, scaled from 0 to 100, representing linear loudness values.
//
// An error will be returned if the [Spec] given has no sample-rate or duration corresponding to the
// data given, as both these values are necessary for deriving the number of samples.
//
// The number of samples returned will be equal to the given maximum number provided, and will be
// padded with 0 values if necessary.
func GetWaveform(ctx context.Context, data []byte, spec *Spec, maxSamples int) ([]byte, error) {
	if spec.AudioSampleRate == 0 || spec.Duration == 0 {
		return nil, fmt.Errorf("no sample-rate or duration for media given")
	}

	in, err := createTempFile(data)
	if err != nil {
		return nil, err
	}

	defer os.Remove(in)

	// Determine number of waveform to take based on duration and sample-rate of original file.
	numSamples := strconv.Itoa(int(float64(spec.AudioSampleRate)*spec.Duration.Seconds()) / maxSamples)
	out, err := ffprobe(ctx,
		"amovie="+in+",asetnsamples="+numSamples+",astats=metadata=1:reset=1",
		"-f", "lavfi",
		"-show_entries", "frame_tags=lavfi.astats.Overall.Peak_level",
	)

	// Get waveform with defined maximum number of samples, and scale these from a range of 0 to 100.
	var samples = make([]byte, 0, maxSamples)
	if f, ok := out["frames"].([]any); ok {
		if len(f) == 0 {
			return nil, fmt.Errorf("no audio frames found in media")
		}
		for i := range f {
			if r, ok := f[i].(map[string]any); ok {
				if t, ok := r["tags"].(map[string]any); ok {
					if v, ok := t["lavfi.astats.Overall.Peak_level"].(string); ok {
						db, err := strconv.ParseFloat(v, 64)
						if err == nil {
							samples = append(samples, byte(math.Pow(10, (db/50))*100))
						}
					}
				}
			}
		}
	}

	return samples, nil
}

var (
	// The default path for storing temporary files.
	tempDir = os.TempDir()
)

// SetTempDirectory sets the global temporary directory used internally by media conversion commands.
func SetTempDirectory(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}

	tempDir = path
	return nil
}

// CreateTempFile creates a temporary file in the pre-defined temporary directory (or the default,
// system-wide temporary directory, if no override value was set) and returns the absolute path for
// the file, or an error if none could be created.
func createTempFile(data []byte) (string, error) {
	f, err := os.CreateTemp(tempDir, "media-*")
	if err != nil {
		return "", fmt.Errorf("failed creating temporary file: %w", err)
	}

	defer f.Close()
	if len(data) > 0 {
		if n, err := f.Write(data); err != nil {
			os.Remove(f.Name())
			return "", fmt.Errorf("failed writing to temporary file: %w", err)
		} else if n < len(data) {
			os.Remove(f.Name())
			return "", fmt.Errorf("failed writing to temporary file: incomplete write, want %d, write %d bytes", len(data), n)
		}
	}

	return f.Name(), nil
}
