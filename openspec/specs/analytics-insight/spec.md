# analytics-insight Specification

## Purpose
TBD - created by archiving change analytics-python-service. Update Purpose after archive.
## Requirements
### Requirement: Trend Insight Discovery

The system SHALL automatically discover trend-related insights.

#### Scenario: Upward trend insight
- **WHEN** a significant upward trend is detected
- **THEN** the system SHALL generate an insight with direction, magnitude, and significance

#### Scenario: Downward trend insight
- **WHEN** a significant downward trend is detected
- **THEN** the system SHALL generate an insight with warning severity

#### Scenario: Trend acceleration
- **WHEN** recent trend slope is 1.5x greater than earlier trend slope
- **THEN** the system SHALL generate an insight about acceleration

#### Scenario: Generate recommendations
- **WHEN** a trend insight is generated
- **THEN** the system SHALL provide context-aware recommendations (e.g., analyze drivers, assess sustainability)

### Requirement: Anomaly Insight Discovery

The system SHALL detect and report anomalous data points.

#### Scenario: Spike detection
- **WHEN** values above Q3 + 1.5*IQR are detected
- **THEN** the system SHALL generate spike anomaly insight with count, max value, and dates

#### Scenario: Dip detection
- **WHEN** values below Q1 - 1.5*IQR are detected
- **THEN** the system SHALL generate dip anomaly insight with count, min value, and dates

#### Scenario: Z-score method
- **WHEN** anomaly detection with method="zscore" is requested
- **THEN** the system SHALL flag values with |z-score| > threshold (default 3.0)

#### Scenario: Anomaly recommendations
- **WHEN** anomaly insights are generated
- **THEN** the system SHALL suggest data validation and root cause analysis

### Requirement: Correlation Insight Discovery

The system SHALL discover and report significant correlations.

#### Scenario: Strong positive correlation
- **WHEN** two variables have correlation > threshold (default 0.7)
- **THEN** the system SHALL generate insight describing the relationship

#### Scenario: Strong negative correlation
- **WHEN** two variables have correlation < -threshold
- **THEN** the system SHALL generate insight noting negative relationship

#### Scenario: Correlation recommendations
- **WHEN** correlation insight is generated
- **THEN** the system SHALL suggest analyzing business context and noting correlation vs causation

### Requirement: Insight Severity Classification

The system SHALL classify insights by severity.

#### Scenario: High severity
- **WHEN** insight involves strong trend (R² > 0.7) or multiple anomalies
- **THEN** the system SHALL classify as "high"

#### Scenario: Medium severity
- **WHEN** insight involves moderate trend (R² > 0.3) or single anomaly
- **THEN** the system SHALL classify as "medium"

#### Scenario: Low severity
- **WHEN** insight involves weak trend or minor changes
- **THEN** the system SHALL classify as "low"

### Requirement: Insight Confidence Scoring

The system SHALL assign confidence scores to insights.

#### Scenario: Statistical confidence
- **WHEN** insight is based on statistical test
- **THEN** confidence SHALL be derived from p-value or R²

#### Scenario: Correlation confidence
- **WHEN** insight is correlation-based
- **THEN** confidence SHALL equal absolute correlation coefficient

### Requirement: Comprehensive Insight Generation

The system SHALL generate all insights for a dataset.

#### Scenario: Multi-metric analysis
- **WHEN** a DataFrame with multiple numerical columns is provided
- **THEN** the system SHALL analyze each column and generate trend and anomaly insights

#### Scenario: Correlation insights
- **WHEN** multiple numerical columns are provided
- **THEN** the system SHALL analyze correlations and generate correlation insights

#### Scenario: Sort by priority
- **WHEN** all insights are generated
- **THEN** the system SHALL sort by severity (high > medium > low) and confidence

### Requirement: Evidence Tracking

The system SHALL include evidence data for each insight.

#### Scenario: Trend evidence
- **WHEN** trend insight is generated
- **THEN** evidence SHALL include slope, R², p-value, and percentage change

#### Scenario: Anomaly evidence
- **WHEN** anomaly insight is generated
- **THEN** evidence SHALL include count, extreme values, and affected dates

#### Scenario: Correlation evidence
- **WHEN** correlation insight is generated
- **THEN** evidence SHALL include correlation coefficient and method used

