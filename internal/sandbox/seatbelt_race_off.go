//go:build darwin && !race

// seatbelt_race_off.go——race 构建判定（非 race 面——同族 seatbelt_race_on.go，
// 定义面=使用面构建上下文对齐）。
package sandbox

const raceInstrumented = false
