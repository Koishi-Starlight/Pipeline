package main

import (
	"bufio"
	"container/ring"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const bufferDrainInterval time.Duration = 10 * time.Second
const bufferSize int = 10

type RingIntBuffer struct {
	r    *ring.Ring
	size int
	m    sync.Mutex
}

func NewRingIntBuffer(size int) *RingIntBuffer {
	return &RingIntBuffer{r: ring.New(size), size: size, m: sync.Mutex{}}
}

func (r *RingIntBuffer) Push(el int) {
	r.m.Lock()
	defer r.m.Unlock()
	r.r.Value = el
	r.r = r.r.Next()
	log.Printf("RingIntBuffer: added value %d to buffer\n", el)
}

func (r *RingIntBuffer) Get() []int {
	r.m.Lock()
	defer r.m.Unlock()
	var output []int
	r.r.Do(func(x interface{}) {
		if x != nil {
			output = append(output, x.(int))
		}
	})
	log.Printf("RingIntBuffer: retrieved buffer values %v\n", output)
	return output
}

type StageInt interface {
	Process(done <-chan bool, input <-chan int) <-chan int
}

type NegativeFilterStage struct{}

func (n *NegativeFilterStage) Process(done <-chan bool, input <-chan int) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		for {
			select {
			case data, ok := <-input:
				if !ok {
					log.Println("NegativeFilterStage: input channel closed")
					return
				}
				if data > 0 {
					log.Printf("NegativeFilterStage: passing value %d\n", data)
					select {
					case output <- data:
					case <-done:
						return
					}
				} else {
					log.Printf("NegativeFilterStage: filtering out value %d\n", data)
				}
			case <-done:
				return
			}
		}
	}()
	return output
}

type SpecialFilterStage struct{}

func (s *SpecialFilterStage) Process(done <-chan bool, input <-chan int) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		for {
			select {
			case data, ok := <-input:
				if !ok {
					log.Println("SpecialFilterStage: input channel closed")
					return
				}
				if data != 0 && data%3 == 0 {
					log.Printf("SpecialFilterStage: passing value %d\n", data)
					select {
					case output <- data:
					case <-done:
						return
					}
				} else {
					log.Printf("SpecialFilterStage: filtering out value %d\n", data)
				}
			case <-done:
				return
			}
		}
	}()
	return output
}

type BufferStage struct {
	buffer *RingIntBuffer
}

func (b *BufferStage) Process(done <-chan bool, input <-chan int) <-chan int {
	output := make(chan int)
	go func() {
		defer close(output)
		for {
			select {
			case data, ok := <-input:
				if !ok {
					log.Println("BufferStage: input channel closed")
					return
				}
				log.Printf("BufferStage: received value %d\n", data)
				b.buffer.Push(data)
			case <-time.After(bufferDrainInterval):
				bufferData := b.buffer.Get()
				if bufferData != nil {
					log.Printf("BufferStage: draining buffer with values %v\n", bufferData)
					for _, data := range bufferData {
						select {
						case output <- data:
						case <-done:
							return
						}
					}
				}
			case <-done:
				return
			}
		}
	}()
	return output
}

type PipelineInt struct {
	stages []StageInt
	done   <-chan bool
}

func NewPipelineInt(done <-chan bool, stages ...StageInt) *PipelineInt {
	return &PipelineInt{done: done, stages: stages}
}

func (p *PipelineInt) Run(source <-chan int) <-chan int {
	var c <-chan int = source
	for index := range p.stages {
		c = p.runStageInt(p.stages[index], c)
	}
	return c
}

func (p *PipelineInt) runStageInt(stage StageInt, sourceChan <-chan int) <-chan int {
	return stage.Process(p.done, sourceChan)
}

func main() {
	log.SetOutput(os.Stdout)
	log.Println("Pipeline: starting up")
	dataSource := func() (<-chan int, <-chan bool) {
		c := make(chan int)
		done := make(chan bool)
		go func() {
			defer close(done)
			scanner := bufio.NewScanner(os.Stdin)
			var data string
			for {
				scanner.Scan()
				data = scanner.Text()
				if strings.EqualFold(data, "exit") {
					fmt.Println("Программа завершила работу!")
					log.Println("DataSource: received exit command")
					return
				}
				i, err := strconv.Atoi(data)
				if err != nil {
					fmt.Println("Программа обрабатывает только целые числа!")
					log.Printf("DataSource: failed to parse input %s\n", data)
					continue
				}
				log.Printf("DataSource: received value %d\n", i)
				c <- i
			}
		}()
		return c, done
	}

	buffer := NewRingIntBuffer(bufferSize)
	negativeFilter := &NegativeFilterStage{}
	specialFilter := &SpecialFilterStage{}
	bufferStage := &BufferStage{buffer: buffer}

	consumer := func(done <-chan bool, c <-chan int) {
		for {
			select {
			case data := <-c:
				log.Printf("Consumer: processed value %d\n", data)
				fmt.Printf("Обработаны данные: %d\n", data)
			case <-done:
				log.Println("Consumer: done channel closed")
				return
			}
		}
	}

	source, done := dataSource()
	pipeline := NewPipelineInt(done, negativeFilter, specialFilter, bufferStage)
	consumer(done, pipeline.Run(source))
	log.Println("Pipeline: shutting down")
}
