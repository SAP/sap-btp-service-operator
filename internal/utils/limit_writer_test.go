package utils

import (
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("limit writer", func() {
	/* tests for limit_writer.go */
	Describe("limited writer", func() {
		It("should write to the underlying writer", func() {
			// Given
			limitedWriter := LimitedWriter{
				W:         nil,
				N:         0,
				Converter: nil,
			}

			// When
			i, err := limitedWriter.Write(nil)
			// Then
			Expect(i).To(Equal(0))
			Expect(err).To(Equal(ErrLimitExceeded))
		})
		It("should return error when N <= 0", func() {
			// Given
			limitedWriter := LimitedWriter{
				W: nil,
				N: 0,
				Converter: func(err error) error {
					if errors.Is(err, ErrLimitExceeded) {
						return fmt.Errorf("the size of the generated secret manifest exceeds the limit of %d bytes", 0)
					}
					return err
				},
			}

			// When
			i, err := limitedWriter.Write(nil)

			// Then
			Expect(i).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("the size of the generated secret manifest exceeds the limit of 0 bytes"))

		})
		It("should return error when N < writeable", func() {
			// Given
			limitedWriter := LimitedWriter{
				W:         nil,
				N:         0,
				Converter: nil,
			}

			// When
			limitedWriter.Write(nil)

			// Then
		})

	})

})
