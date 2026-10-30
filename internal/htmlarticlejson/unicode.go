package htmlarticlejson

import (
	"context"

	"github.com/sidsharma96/concordance/internal/jsonunicode"
)

func surrogates(ctx context.Context, body []byte) error {
	err := jsonunicode.CheckSurrogates(ctx, body)
	if err == jsonunicode.ErrInvalid {
		return ErrInvalid
	}
	return err
}
