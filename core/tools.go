//go:build tools
// +build tools

package core

import (
	_ "github.com/golang-cz/ringbuf"
	_ "github.com/google/uuid"
	_ "github.com/openai/openai-go/v3"
	_ "github.com/stretchr/testify"
)