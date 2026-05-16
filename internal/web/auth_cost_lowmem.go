//go:build mips || mipsle

package web

// bcryptCost на MIPS softfloat снижен до 10 для UX (login ≤ 2s vs 6s при cost=12).
// См. BEHAVIOR_SPEC.md §Web UI — Auth.
const bcryptCost = 10
