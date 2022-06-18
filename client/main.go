package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

func main() {

	fmt.Println("creating client")

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

				if err != nil {
					fmt.Println("error creating payment", err.Error())
					return
				}

				b, _ := io.ReadAll(response.Body)
				var p struct {
					ID string `json:"id"`
				}

				if err := json.Unmarshal(b, &p); err != nil {
					fmt.Println("error payment response", err.Error())
					return
				}

				fmt.Println("payment id", p.ID, "created successfully")
			}()
		default:

		}
	}
}
