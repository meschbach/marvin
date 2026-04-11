## ADDED Requirements

### Requirement: RetryBlock accessor validates configured values
The RetryBlock accessor methods (MaxRetriesValue, InitialIntervalValue, MaxIntervalValue) SHALL return an error when the user has configured a value that is numerically non-positive.

#### Scenario: MaxRetries configured as zero
- **GIVEN** RetryBlock has MaxRetries set to 0
- **WHEN** MaxRetriesValue() is called
- **THEN** an error is returned indicating MaxRetries must be positive

#### Scenario: MaxRetries configured as negative
- **GIVEN** RetryBlock has MaxRetries set to -1
- **WHEN** MaxRetriesValue() is called
- **THEN** an error is returned indicating MaxRetries must be positive

#### Scenario: MaxRetries not configured (nil)
- **GIVEN** RetryBlock has MaxRetries as nil
- **WHEN** MaxRetriesValue() is called
- **THEN** returns DefaultMaxRetries (3) without error

#### Scenario: InitialInterval configured as zero
- **GIVEN** RetryBlock has InitialInterval set to 0s
- **WHEN** InitialIntervalValue() is called
- **THEN** an error is returned indicating InitialInterval must be positive

#### Scenario: InitialInterval configured as negative
- **GIVEN** RetryBlock has InitialInterval set to -5s
- **WHEN** InitialIntervalValue() is called
- **THEN** an error is returned indicating InitialInterval must be positive

#### Scenario: InitialInterval not configured (nil)
- **GIVEN** RetryBlock has InitialInterval as nil
- **WHEN** InitialIntervalValue() is called
- **THEN** returns DefaultInitialInterval (1s) without error

#### Scenario: MaxInterval configured as zero
- **GIVEN** RetryBlock has MaxInterval set to 0s
- **WHEN** MaxIntervalValue() is called
- **THEN** an error is returned indicating MaxInterval must be positive

#### Scenario: MaxInterval configured as negative
- **GIVEN** RetryBlock has MaxInterval set to -10s
- **WHEN** MaxIntervalValue() is called
- **THEN** an error is returned indicating MaxInterval must be positive

#### Scenario: MaxInterval not configured (nil)
- **GIVEN** RetryBlock has MaxInterval as nil
- **WHEN** MaxIntervalValue() is called
- **THEN** returns DefaultMaxInterval (30s) without error