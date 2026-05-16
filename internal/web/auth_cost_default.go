//go:build !mips && !mipsle

package web

// bcryptCost на не-MIPS архитектурах: 12 (рекомендуемый минимум для production).
const bcryptCost = 12
