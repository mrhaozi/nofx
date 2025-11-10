package market

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestIdentifyWyckoffSignals 测试维科夫信号识别函数
func TestIdentifyWyckoffSignals(t *testing.T) {
	// 模拟K线数据用于测试不同的维科夫信号
	mockKlines := []Kline{
		{Open: 100000, High: 101000, Low: 99000, Close: 100500, Volume: 1000},
		{Open: 100500, High: 102000, Low: 100000, Close: 101500, Volume: 1200},
		{Open: 101500, High: 103000, Low: 101000, Close: 102500, Volume: 1500},
		{Open: 102500, High: 104000, Low: 102000, Close: 103500, Volume: 1800},
		{Open: 103500, High: 105000, Low: 103000, Close: 104500, Volume: 2000}, // 上升趋势
		{Open: 104500, High: 106000, Low: 104000, Close: 105500, Volume: 2200},
		{Open: 105500, High: 107000, Low: 105000, Close: 106500, Volume: 2500},
		{Open: 106500, High: 108000, Low: 106000, Close: 107500, Volume: 2800},
		{Open: 107500, High: 109000, Low: 107000, Close: 108500, Volume: 3000},
		{Open: 108500, High: 110000, Low: 108000, Close: 109500, Volume: 3200}, // 高点
		{Open: 109500, High: 109800, Low: 107000, Close: 108000, Volume: 3500}, // UTAD模式：上影线长
		{Open: 108000, High: 108500, Low: 106000, Close: 107000, Volume: 2800},
		{Open: 107000, High: 107500, Low: 105000, Close: 106000, Volume: 2500},
		{Open: 106000, High: 106500, Low: 104000, Close: 105000, Volume: 2200},
		{Open: 105000, High: 105500, Low: 103000, Close: 104000, Volume: 2000},
		{Open: 104000, High: 104500, Low: 102000, Close: 103000, Volume: 1800},
		{Open: 103000, High: 103500, Low: 101000, Close: 102000, Volume: 1500},
		{Open: 102000, High: 102500, Low: 100000, Close: 101000, Volume: 1200},
		{Open: 101000, High: 101500, Low: 99000, Close: 100000, Volume: 1000},
		{Open: 100000, High: 100200, Low: 98000, Close: 99500, Volume: 1200}, // Spring模式：下影线长
		{Open: 99500, High: 100500, Low: 98500, Close: 100000, Volume: 1100},
		{Open: 100000, High: 102000, Low: 99500, Close: 101500, Volume: 1500},  // SOS模式：大阳线
		{Open: 101500, High: 101800, Low: 100000, Close: 100500, Volume: 900},  // TEST模式：小实体
		{Open: 100500, High: 104000, Low: 100000, Close: 103500, Volume: 3000}, // CLIMAX：大波动
		{Open: 103500, High: 103500, Low: 101000, Close: 101500, Volume: 1600}, // SOW模式：大阴线
		{Open: 101500, High: 102000, Low: 101000, Close: 101800, Volume: 1300},
		{Open: 101800, High: 105000, Low: 101500, Close: 104500, Volume: 2200}, // BREAKOUT：突破前高
		{Open: 104500, High: 104500, Low: 102000, Close: 102500, Volume: 1800}, // BREAKDOWN：跌破前低
	}

	// 测试市场阶段识别
	phase := identifyMarketPhase(mockKlines)
	fmt.Printf("识别的市场阶段: %s\n", phase)

	// 测试维科夫信号检测
	signals := detectWyckoffSignals(mockKlines)
	fmt.Printf("检测到的维科夫信号: %v\n", signals)

	// 测试成交量模式分析
	volumePattern := analyzeVolumePattern(mockKlines)
	fmt.Printf("成交量模式: %s\n", volumePattern)

	// 测试价格行为识别
	priceAction := identifyPriceAction(mockKlines)
	fmt.Printf("价格行为: %s\n", priceAction)

	// 验证结果
	validPhases := []string{"accumulation", "distribution", "uptrend", "downtrend", "consolidation"}
	isValidPhase := false
	for _, validPhase := range validPhases {
		if phase == validPhase {
			isValidPhase = true
			break
		}
	}
	if !isValidPhase {
		t.Errorf("识别的市场阶段无效: %s", phase)
	}

	// 验证信号
	validSignals := []string{"Spring", "UTAD", "SOS", "SOW", "CLIMAX", "TEST", "BREAKOUT", "BREAKDOWN"}
	for _, signal := range signals {
		isValidSignal := false
		for _, validSignal := range validSignals {
			if signal == validSignal {
				isValidSignal = true
				break
			}
		}
		if !isValidSignal {
			t.Errorf("检测到的信号无效: %s", signal)
		}
	}

	// 验证成交量模式
	validVolumePatterns := []string{"high_volume", "low_volume", "normal_volume", "divergence"}
	isValidVolumePattern := false
	for _, validPattern := range validVolumePatterns {
		if volumePattern == validPattern {
			isValidVolumePattern = true
			break
		}
	}
	if !isValidVolumePattern {
		t.Errorf("成交量模式无效: %s", volumePattern)
	}

	// 验证价格行为
	validPriceActions := []string{"breakout", "breakdown", "false_move", "consolidation", "trending"}
	isValidPriceAction := false
	for _, validAction := range validPriceActions {
		if priceAction == validAction {
			isValidPriceAction = true
			break
		}
	}
	if !isValidPriceAction {
		t.Errorf("价格行为无效: %s", priceAction)
	}

	fmt.Println("维科夫信号识别测试通过!")
}

// TestWyckoffSignalJSON 测试维科夫信号数据的JSON序列化
func TestWyckoffSignalJSON(t *testing.T) {
	wyckoffData := &WyckoffSignalData{
		Phase:          "distribution",
		SignalsPresent: []string{"UTAD", "SOW"},
		VolumePattern:  "effort_no_result",
		PriceAction:    "false_breakout",
	}

	jsonData, err := json.MarshalIndent(wyckoffData, "", "  ")
	if err != nil {
		t.Errorf("JSON序列化失败: %v", err)
	}

	fmt.Printf("维科夫信号数据JSON格式:\n%s\n", string(jsonData))

	// 验证JSON结构
	var decoded WyckoffSignalData
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Errorf("JSON反序列化失败: %v", err)
	}

	if decoded.Phase != wyckoffData.Phase {
		t.Errorf("Phase不匹配: 期望 %s，实际 %s", wyckoffData.Phase, decoded.Phase)
	}
	if len(decoded.SignalsPresent) != len(wyckoffData.SignalsPresent) {
		t.Errorf("SignalsPresent长度不匹配: 期望 %d，实际 %d", len(wyckoffData.SignalsPresent), len(decoded.SignalsPresent))
	}
	if decoded.VolumePattern != wyckoffData.VolumePattern {
		t.Errorf("VolumePattern不匹配: 期望 %s，实际 %s", wyckoffData.VolumePattern, decoded.VolumePattern)
	}
	if decoded.PriceAction != wyckoffData.PriceAction {
		t.Errorf("PriceAction不匹配: 期望 %s，实际 %s", wyckoffData.PriceAction, decoded.PriceAction)
	}
}

// TestIndividualWyckoffPatterns 测试各个维科夫模式识别
func TestIndividualWyckoffPatterns(t *testing.T) {
	// 测试Spring模式
	springKlines := []Kline{
		{Open: 100000, High: 101000, Low: 99000, Close: 100500},
		{Open: 100500, High: 100800, Low: 98000, Close: 100200}, // 长下影线
		{Open: 100200, High: 101000, Low: 99500, Close: 100800}, // 反弹
	}
	if !isSpringPattern(springKlines) {
		t.Errorf("未能正确识别Spring模式")
	}

	// 测试UTAD模式
	utadKlines := []Kline{
		{Open: 105000, High: 106000, Low: 104500, Close: 105500},
		{Open: 105500, High: 108000, Low: 105000, Close: 105800}, // 长上影线
		{Open: 105800, High: 106000, Low: 105000, Close: 105200}, // 回落
	}
	if !isUTADPattern(utadKlines) {
		t.Errorf("未能正确识别UTAD模式")
	}

	// 测试SOS模式
	sosKlines := []Kline{
		{Open: 100000, High: 100500, Low: 99500, Close: 100200},
		{Open: 100200, High: 105000, Low: 100000, Close: 104500}, // 大阳线
	}
	if !isSOSPattern(sosKlines) {
		t.Errorf("未能正确识别SOS模式")
	}

	// 测试SOW模式
	sowKlines := []Kline{
		{Open: 105000, High: 105500, Low: 104500, Close: 105200},
		{Open: 105200, High: 105500, Low: 101000, Close: 101500}, // 大阴线
	}
	if !isSOWPattern(sowKlines) {
		t.Errorf("未能正确识别SOW模式")
	}

	// 测试CLIMAX模式
	climaxKlines := []Kline{
		{Open: 100000, High: 100500, Low: 99500, Close: 100000},
		{Open: 100000, High: 108000, Low: 92000, Close: 104000}, // 更大波动（16% > 5%）
	}
	if !isClimaxPattern(climaxKlines) {
		t.Errorf("未能正确识别CLIMAX模式")
	}

	// 测试TEST模式
	testKlines := []Kline{
		{Open: 101000, High: 102000, Low: 100000, Close: 101500},
		{Open: 101500, High: 101800, Low: 101200, Close: 101600}, // 小实体
	}
	if !isTestPattern(testKlines) {
		t.Errorf("未能正确识别TEST模式")
	}

	fmt.Println("各个维科夫模式识别测试通过!")
}
