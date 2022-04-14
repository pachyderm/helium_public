package controlloop

import "context"

type Manager struct{}

type ControlFunc func(context.Context) error

func (m Manager) Add(ControlFunc) {}

func (m Manager) Run(ctx context.Context) {}
