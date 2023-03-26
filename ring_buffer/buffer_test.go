package ring_buffer

import "testing"

func TestRingBuffer_Add(t *testing.T) {
	t.Run("fill ring buffer with digits until it loops, and test that it works", func(t *testing.T) {
		ringBuffer := New(10)

		for i := 0; i < 20; i++ {
			ringBuffer.Add([]int16{int16(i)})
		}

		expected := []int16{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
		actual := ringBuffer.Read()

		for i := 0; i < 10; i++ {
			if expected[i] != actual[i] {
				t.Errorf("expected %d, got %d", expected[i], actual[i])
			}
		}
	})
}
