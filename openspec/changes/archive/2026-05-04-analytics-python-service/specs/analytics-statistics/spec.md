# Analytics Statistics Capability

数理统计能力 - 提供描述统计、分布检验、相关性分析等统计计算。

## ADDED Requirements

### Requirement: Descriptive Statistics

The system SHALL compute descriptive statistics for numerical data series.

#### Scenario: Compute basic statistics
- **WHEN** a numerical data series is provided
- **THEN** the system SHALL return count, mean, median, standard deviation, min, max, Q25, Q75, and IQR

#### Scenario: Compute distribution shape
- **WHEN** a numerical data series with sufficient size (>3) is provided
- **THEN** the system SHALL return skewness and kurtosis

#### Scenario: Handle empty or invalid data
- **WHEN** an empty series or series with all NA values is provided
- **THEN** the system SHALL return an error with appropriate message

### Requirement: Distribution Analysis

The system SHALL perform normality tests on numerical data.

#### Scenario: Shapiro-Wilk test for small samples
- **WHEN** a data series with less than 5000 observations is provided
- **THEN** the system SHALL use Shapiro-Wilk test for normality

#### Scenario: Kolmogorov-Smirnov test for large samples
- **WHEN** a data series with 5000 or more observations is provided
- **THEN** the system SHALL use KS test for normality

#### Scenario: Return test results
- **WHEN** a normality test is performed
- **THEN** the system SHALL return test statistic, p-value, and whether the data is normally distributed (p > 0.05)

### Requirement: Correlation Analysis

The system SHALL compute correlation coefficients between numerical variables.

#### Scenario: Pearson correlation
- **WHEN** correlation analysis is requested with method="pearson"
- **THEN** the system SHALL compute Pearson correlation coefficient for all numerical variable pairs

#### Scenario: Spearman correlation
- **WHEN** correlation analysis is requested with method="spearman"
- **THEN** the system SHALL compute Spearman rank correlation

#### Scenario: Identify significant correlations
- **WHEN** correlation analysis is performed
- **THEN** the system SHALL return pairs with correlation above threshold (default 0.7)

### Requirement: Percentile Calculation

The system SHALL compute percentiles for numerical data.

#### Scenario: Default percentiles
- **WHEN** percentile calculation is requested without specific percentiles
- **THEN** the system SHALL return P1, P5, P10, P25, P50, P75, P90, P95, P99

#### Scenario: Custom percentiles
- **WHEN** specific percentiles are requested
- **THEN** the system SHALL return only the requested percentiles

### Requirement: Hypothesis Testing

The system SHALL perform common hypothesis tests.

#### Scenario: Independent t-test
- **WHEN** two numerical samples are provided for comparison
- **THEN** the system SHALL perform independent t-test and return statistic, p-value, and significance

#### Scenario: Chi-square test
- **WHEN** a contingency table is provided
- **THEN** the system SHALL perform chi-square test of independence
