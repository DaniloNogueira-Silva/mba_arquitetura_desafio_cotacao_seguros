package main

import (
	"encoding/json"
	"fmt"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/quotation"
)

func quoteBodies(distinct int) ([][]byte, error) {
	models := []string{"Onix 1.0", "HB20 1.6", "Gol 1.0", "Argo 1.3", "Kwid 1.0"}

	bodies := make([][]byte, 0, distinct)
	for i := 0; i < distinct; i++ {
		body, err := json.Marshal(quotation.Request{
			Driver: quotation.Driver{
				Document:  fmt.Sprintf("%011d", 12345678900+i),
				BirthYear: 1970 + i%30,
			},
			Vehicle: quotation.Vehicle{
				Plate:      fmt.Sprintf("LDG%04d", i),
				Model:      models[i%len(models)],
				Year:       2015 + i%10,
				ValueCents: 4500000 + int64(i%20)*250000,
			},
			Coverage: "comprehensive",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build the quote body: %w", err)
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}
