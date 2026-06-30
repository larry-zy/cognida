# Analytics Trend Capability

趋势分析能力 - 提供线性趋势检测、季节分解、增长率计算等时序分析。

## ADDED Requirements

### Requirement: Linear Trend Analysis

The system SHALL analyze linear trends in time series data.

#### Scenario: Detect upward trend
- **WHEN** a time series shows significant upward trend (p < 0.05, slope > 0)
- **THEN** the system SHALL identify trend direction as "up" with confidence level based on p-value

#### Scenario: Detect downward trend
- **WHEN** a time series shows significant downward trend (p < 0.05, slope < 0)
- **THEN** the system SHALL identify trend direction as "down" with confidence level

#### Scenario: No significant trend
- **WHEN** a time series has no significant trend (p >= 0.05)
- **THEN** the system SHALL identify trend direction as "flat"

#### Scenario: Trend strength
- **WHEN** linear regression is performed
- **THEN** the system SHALL categorize strength as "strong" (R² > 0.7), "moderate" (R² > 0.3), or "weak"

#### Scenario: Return regression metrics
- **WHEN** trend analysis is performed
- **THEN** the system SHALL return slope, R², p-value, and confidence interval

### Requirement: Trend Forecasting

The system SHALL generate short-term forecasts based on linear trend.

#### Scenario: Linear forecast
- **WHEN** forecast is requested with N steps
- **THEN** the system SHALL return N predicted values using the linear regression model

### Requirement: Seasonality Analysis

The system SHALL decompose time series into trend, seasonal, and residual components.

#### Scenario: Seasonal decomposition
- **WHEN** a time series with sufficient data is provided
- **THEN** the system SHALL return trend, seasonal, and residual components

#### Scenario: Detect seasonality strength
- **WHEN** seasonal decomposition is performed
- **THEN** the system SHALL calculate seasonality strength (seasonal variance / total variance)

#### Scenario: Detect period
- **WHEN** period is not specified
- **THEN** the system SHALL auto-detect period using ACF (default max 50)

#### Scenario: Insufficient data
- **WHEN** data length is less than 2 * period
- **THEN** the system SHALL return has_seasonality=false with empty components

### Requirement: Growth Rate Analysis

The system SHALL calculate various growth rate metrics.

#### Scenario: Period-over-period growth
- **WHEN** period-over-period growth is requested
- **THEN** the system SHALL calculate (current - prior) / prior for each period

#### Scenario: Year-over-year growth
- **WHEN** YoY growth is requested
- **THEN** the system SHALL calculate growth rate vs same period in previous cycle

#### Scenario: CAGR calculation
- **WHEN** compound annual growth rate is requested
- **THEN** the system SHALL calculate (end/start)^(1/n) - 1 where n is number of periods

### Requirement: Moving Average

The system SHALL calculate moving averages for smoothing.

#### Scenario: Simple moving average
- **WHEN** SMA is requested with window W
- **THEN** the system SHALL return rolling average with window W

#### Scenario: Exponential moving average
- **WHEN** EMA is requested with span S
- **THEN** the system SHALL return exponentially weighted moving average

#### Scenario: Double exponential smoothing
- **WHEN** double EMA is requested
- **THEN** the system SHALL return Holt's method smoothed values
