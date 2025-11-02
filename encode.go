package blurhash

import (
	"image"
	"image/draw"
	"math"
	"strings"

	"github.com/bbrks/go-blurhash/base83"
)

const (
	minComponents = 1
	maxComponents = 9
)

// Encode returns the blurhash for the given image.
func Encode(xComponents, yComponents int, img image.Image) (hash string, err error) {
	if xComponents < minComponents || xComponents > maxComponents ||
		yComponents < minComponents || yComponents > maxComponents {
		return "", ErrInvalidComponents
	}

	b := strings.Builder{}

	sizeFlag := (xComponents - 1) + (yComponents-1)*9
	sizeFlagEncoded, err := base83.Encode(sizeFlag, 1)
	if err != nil {
		return "", err
	}

	_, err = b.WriteString(sizeFlagEncoded)
	if err != nil {
		return "", err
	}

	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	factorsCount := xComponents * yComponents

	var cosX = make([]float64, width*factorsCount)
	var cosY = make([]float64, height*factorsCount)

	for i := 0; i < width; i++ {
		for x := 0; x < xComponents; x++ {
			weight := math.Cos(math.Pi * float64(x*i) / float64(width))
			for y := 0; y < yComponents; y++ {
				cosX[i*factorsCount+y*xComponents+x] = weight
			}
		}
	}

	for i := 0; i < height; i++ {
		for y := 0; y < yComponents; y++ {
			weight := math.Cos(math.Pi * float64(y*i) / float64(height))
			for x := 0; x < xComponents; x++ {
				cosY[i*factorsCount+y*xComponents+x] = weight
			}
		}
	}

	factors := make([][3]float64, factorsCount)
	multiplyBasisFunction(factors, factorsCount, img, width, height, cosX, cosY)

	maximumValue := 0.0
	if xComponents*yComponents-1 > 0 {
		actualMaximumValue := 0.0

		for i := 0; i < factorsCount; i++ {
			if i == 0 {
				continue
			}

			actualMaximumValue = math.Max(math.Abs(factors[i][0]), actualMaximumValue)
			actualMaximumValue = math.Max(math.Abs(factors[i][1]), actualMaximumValue)
			actualMaximumValue = math.Max(math.Abs(factors[i][2]), actualMaximumValue)
		}

		quantisedMaximumValue := math.Max(0, math.Min(82, math.Floor(actualMaximumValue*166-0.5)))
		maximumValue = (quantisedMaximumValue + 1) / 166
		str, err := base83.Encode(int(quantisedMaximumValue), 1)
		if err != nil {
			return "", err
		}
		b.WriteString(str)
	} else {
		maximumValue = 1
		str, err := base83.Encode(0, 1)
		if err != nil {
			return "", err
		}
		b.WriteString(str)
	}

	dc := factors[0]
	str, err := base83.Encode(encodeDC(dc[0], dc[1], dc[2]), 4)
	if err != nil {
		return "", err
	}
	b.WriteString(str)

	for i := 0; i < factorsCount; i++ {
		if i == 0 {
			continue
		}
		str, err := base83.Encode(encodeAC(factors[i][0], factors[i][1], factors[i][2], maximumValue), 2)
		if err != nil {
			return "", err
		}
		b.WriteString(str)
	}

	return b.String(), nil
}

func encodeDC(r, g, b float64) int {
	return (linearTosRGB(r) << 16) + (linearTosRGB(g) << 8) + linearTosRGB(b)
}

func encodeAC(r, g, b, maximumValue float64) int {
	quantR := math.Max(0, math.Min(18, math.Floor(signPow(r/maximumValue, 0.5)*9+9.5)))
	quantG := math.Max(0, math.Min(18, math.Floor(signPow(g/maximumValue, 0.5)*9+9.5)))
	quantB := math.Max(0, math.Min(18, math.Floor(signPow(b/maximumValue, 0.5)*9+9.5)))

	return int(quantR*19*19 + quantG*19 + quantB)
}

func multiplyBasisFunction(factors [][3]float64, factorsCount int, img image.Image, width, height int, cosX, cosY []float64) {
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		bounds := img.Bounds()
		nrgba = image.NewNRGBA(bounds)
		draw.Draw(nrgba, bounds, img, bounds.Min, draw.Src)
	}

	stride := nrgba.Stride
	pix := nrgba.Pix

	for y := 0; y < height; y++ {
		cosYLocal := cosY[y*factorsCount:]
		x := 0

		for ; x < width-3; x += 4 {
			offset := y*stride + x*4
			cosXLocal := cosX[x*factorsCount:]

			var pixel10 = [3]float64{sRGBToLinearCache[int(pix[offset+0])], sRGBToLinearCache[int(pix[offset+1])], sRGBToLinearCache[int(pix[offset+2])]}
			var pixel11 = [3]float64{sRGBToLinearCache[int(pix[offset+4])], sRGBToLinearCache[int(pix[offset+5])], sRGBToLinearCache[int(pix[offset+6])]}
			var pixel12 = [3]float64{sRGBToLinearCache[int(pix[offset+8])], sRGBToLinearCache[int(pix[offset+9])], sRGBToLinearCache[int(pix[offset+10])]}
			var pixel13 = [3]float64{sRGBToLinearCache[int(pix[offset+12])], sRGBToLinearCache[int(pix[offset+13])], sRGBToLinearCache[int(pix[offset+14])]}

			for i := 0; i < factorsCount; i++ {
				basis0 := cosYLocal[i] * cosXLocal[i]
				basis1 := cosYLocal[i] * cosXLocal[i+factorsCount]
				basis2 := cosYLocal[i] * cosXLocal[i+2*factorsCount]
				basis3 := cosYLocal[i] * cosXLocal[i+3*factorsCount]

				factors[i][0] += basis0*pixel10[0] + basis1*pixel11[0] + basis2*pixel12[0] + basis3*pixel13[0]
				factors[i][1] += basis0*pixel10[1] + basis1*pixel11[1] + basis2*pixel12[1] + basis3*pixel13[1]
				factors[i][2] += basis0*pixel10[2] + basis1*pixel11[2] + basis2*pixel12[2] + basis3*pixel13[2]
			}
		}

		for ; x < width; x++ {
			offset := y*stride + x*4
			cosXLocal := cosX[x*factorsCount:]

			pixel := [3]float64{
				sRGBToLinearCache[int(pix[offset+0])],
				sRGBToLinearCache[int(pix[offset+1])],
				sRGBToLinearCache[int(pix[offset+3])],
			}

			for i := 0; i < factorsCount; i++ {
				basis := cosYLocal[i] * cosXLocal[i]

				factors[i][0] += basis * pixel[0]
				factors[i][1] += basis * pixel[1]
				factors[i][2] += basis * pixel[2]
			}
		}
	}

	for i := 0; i < factorsCount; i++ {
		normalisation := 2.0
		if i == 0 {
			normalisation = 1.0
		}

		scale := normalisation / float64(width*height)

		factors[i][0] *= scale
		factors[i][1] *= scale
		factors[i][2] *= scale
	}
}
