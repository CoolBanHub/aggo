package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// 场景1：数学计算工具
type CalculatorParams struct {
	Operation string  `json:"operation" jsonschema:"description=运算类型: add(加法)\, subtract(减法)\, multiply(乘法)\, divide(除法),required,enum=add,enum=subtract,enum=multiply,enum=divide"`
	A         float64 `json:"a" jsonschema:"description=第一个数字,required"`
	B         float64 `json:"b" jsonschema:"description=第二个数字,required"`
}

// 场景2：天气查询工具
type WeatherParams struct {
	City string `json:"city" jsonschema:"description=要查询的城市名称,required"`
	Date string `json:"date,omitempty" jsonschema:"description=查询日期，格式为YYYY-MM-DD，不填则查询今天"`
}

// 场景3：时间日期工具
type TimeParams struct {
	Operation string `json:"operation" jsonschema:"description=操作类型: current(当前时间)\, format(格式化时间)\, diff(时间差),required,enum=current,enum=format,enum=diff"`
	Format    string `json:"format,omitempty" jsonschema:"description=时间格式，如 2006-01-02 15:04:05"`
	Time1     string `json:"time1,omitempty" jsonschema:"description=第一个时间点"`
	Time2     string `json:"time2,omitempty" jsonschema:"description=第二个时间点"`
}

// GetCalculatorTool 获取计算器工具
func GetCalculatorTool() []tool.BaseTool {
	name := "calculator"
	desc := "执行基本的数学运算，包括加法、减法、乘法和除法"
	t, _ := utils.InferTool(name, desc, calculateOperation)
	return []tool.BaseTool{t}
}

// GetWeatherTool 获取天气查询工具
func GetWeatherTool() []tool.BaseTool {
	name := "weather_query"
	desc := "查询指定城市的天气信息，可以查询今天或指定日期的天气"
	t, _ := utils.InferTool(name, desc, queryWeather)
	return []tool.BaseTool{t}
}

// GetTimeTool 获取时间工具
func GetTimeTool() []tool.BaseTool {
	name := "time_tool"
	desc := "处理时间相关操作，包括获取当前时间、格式化时间、计算时间差"
	t, _ := utils.InferTool(name, desc, handleTime)
	return []tool.BaseTool{t}
}

// calculateOperation 执行数学运算
func calculateOperation(ctx context.Context, params CalculatorParams) (interface{}, error) {
	var result float64
	var operation string

	switch params.Operation {
	case "add":
		result = params.A + params.B
		operation = "加法"
	case "subtract":
		result = params.A - params.B
		operation = "减法"
	case "multiply":
		result = params.A * params.B
		operation = "乘法"
	case "divide":
		if params.B == 0 {
			return nil, fmt.Errorf("除数不能为0")
		}
		result = params.A / params.B
		operation = "除法"
	default:
		return nil, fmt.Errorf("不支持的运算类型: %s", params.Operation)
	}

	return map[string]interface{}{
		"operation": operation,
		"a":         params.A,
		"b":         params.B,
		"result":    result,
		"formula":   fmt.Sprintf("%.2f %s %.2f = %.2f", params.A, operation, params.B, result),
	}, nil
}

// queryWeather 查询天气信息（模拟数据）
func queryWeather(ctx context.Context, params WeatherParams) (interface{}, error) {
	if params.City == "" {
		return nil, fmt.Errorf("城市名称不能为空")
	}

	// 模拟天气数据
	weatherData := map[string]map[string]interface{}{
		"北京": {
			"temperature": "15-25°C",
			"weather":     "晴转多云",
			"wind":        "北风3-4级",
			"humidity":    "45%",
			"aqi":         "良",
		},
		"上海": {
			"temperature": "18-28°C",
			"weather":     "多云",
			"wind":        "东南风2-3级",
			"humidity":    "65%",
			"aqi":         "优",
		},
		"广州": {
			"temperature": "22-32°C",
			"weather":     "晴",
			"wind":        "南风1-2级",
			"humidity":    "75%",
			"aqi":         "良",
		},
		"深圳": {
			"temperature": "23-31°C",
			"weather":     "晴",
			"wind":        "东南风2级",
			"humidity":    "70%",
			"aqi":         "优",
		},
	}

	data, exists := weatherData[params.City]
	if !exists {
		// 如果城市不在预设列表中，返回默认数据
		data = map[string]interface{}{
			"temperature": "20-28°C",
			"weather":     "晴",
			"wind":        "微风",
			"humidity":    "60%",
			"aqi":         "良",
		}
	}

	queryDate := params.Date
	if queryDate == "" {
		queryDate = time.Now().Format("2006-01-02")
	}

	return map[string]interface{}{
		"city":   params.City,
		"date":   queryDate,
		"data":   data,
		"source": "模拟天气数据",
	}, nil
}

// handleTime 处理时间相关操作
func handleTime(ctx context.Context, params TimeParams) (interface{}, error) {
	switch params.Operation {
	case "current":
		// 获取当前时间
		now := time.Now()
		format := params.Format
		if format == "" {
			format = "2006-01-02 15:04:05"
		}
		return map[string]interface{}{
			"operation": "获取当前时间",
			"timestamp": now.Unix(),
			"formatted": now.Format(format),
			"timezone":  now.Location().String(),
			"year":      now.Year(),
			"month":     int(now.Month()),
			"day":       now.Day(),
			"hour":      now.Hour(),
			"minute":    now.Minute(),
			"second":    now.Second(),
			"weekday":   now.Weekday().String(),
		}, nil

	case "format":
		// 格式化时间
		if params.Time1 == "" {
			return nil, fmt.Errorf("time1参数不能为空")
		}
		// 尝试解析时间
		t, err := time.Parse("2006-01-02 15:04:05", params.Time1)
		if err != nil {
			// 尝试其他格式
			t, err = time.Parse("2006-01-02", params.Time1)
			if err != nil {
				return nil, fmt.Errorf("无法解析时间: %v", err)
			}
		}
		format := params.Format
		if format == "" {
			format = "2006-01-02 15:04:05"
		}
		return map[string]interface{}{
			"operation": "格式化时间",
			"input":     params.Time1,
			"formatted": t.Format(format),
			"timestamp": t.Unix(),
		}, nil

	case "diff":
		// 计算时间差
		if params.Time1 == "" || params.Time2 == "" {
			return nil, fmt.Errorf("time1和time2参数不能为空")
		}
		t1, err := time.Parse("2006-01-02 15:04:05", params.Time1)
		if err != nil {
			t1, err = time.Parse("2006-01-02", params.Time1)
			if err != nil {
				return nil, fmt.Errorf("无法解析time1: %v", err)
			}
		}
		t2, err := time.Parse("2006-01-02 15:04:05", params.Time2)
		if err != nil {
			t2, err = time.Parse("2006-01-02", params.Time2)
			if err != nil {
				return nil, fmt.Errorf("无法解析time2: %v", err)
			}
		}
		diff := t2.Sub(t1)
		return map[string]interface{}{
			"operation": "计算时间差",
			"time1":     params.Time1,
			"time2":     params.Time2,
			"duration":  diff.String(),
			"hours":     diff.Hours(),
			"minutes":   diff.Minutes(),
			"seconds":   diff.Seconds(),
		}, nil

	default:
		return nil, fmt.Errorf("不支持的操作类型: %s", params.Operation)
	}
}
