## MODIFIED Requirements

### Requirement: Local program tool definition
Local programs are executables available to the system as tools.

#### Scenario: Local program with labeled block syntax
- **GIVEN** HCL configuration with local_program block using label
- **WHEN** the configuration is parsed
- **THEN** the labeled block name becomes the program identifier and the `program` field specifies the executable path

#### Scenario: Local program missing program field
- **GIVEN** local_program block without a `program` field
- **WHEN** the configuration is parsed
- **THEN** an error is returned indicating the program field is required

### Requirement: Graceful shutdown handling
The CLI supports graceful shutdown via trap-able signals.

#### Scenario: SIGTERM signal handling
- **WHEN** the process receives SIGTERM signal
- **THEN** the system performs graceful shutdown, completing in-progress operations before exiting

#### Scenario: SIGINT signal handling
- **WHEN** the process receives SIGINT signal (Ctrl+C)
- **THEN** the system performs graceful shutdown, completing in-progress operations before exiting

**Note**: SIGSTOP is not trap-able and should not be used for graceful shutdown.