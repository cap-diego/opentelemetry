package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

func main() {

	fmt.Println("creating clients")
	var wg sync.WaitGroup

	var count = 16
	wg.Add(count)
	for i := 0; i < count; i++ {
		go doWork()
	}

	wg.Wait()
}

func doWork() {
	t := time.NewTicker(500 * time.Millisecond)

	for {
		select {
		case <-t.C:
			go func() {
				cardID := rand.Int()
				amount := rand.Intn(5000)

				fmt.Println("sending payment card_id", cardID, "and amount", amount)

				response, err := http.Post("http://localhost:9000/api/payment",
					"application/json",
					strings.NewReader(fmt.Sprintf(`{"card_id":"%d", "amount":"%d"}`, cardID, amount)))

				if err != nil || response.StatusCode != http.StatusOK {
					fmt.Println("error creating payment")
					return
				}

				b, _ := io.ReadAll(response.Body)
				var p struct {
					ID string `json:"id"`
				}

				if err := json.Unmarshal(b, &p); err != nil {
					fmt.Println("error payment response", err.Error(), "body", b)
					return
				}

				fmt.Println("payment id", p.ID, "created successfully")
			}()

		default:

		}
	}

}
