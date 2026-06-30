## ADDED Requirements

### Requirement: Components Directory Uses Plural Naming
The frontend source code SHALL use `components/` (plural) directory name, following Vue.js community conventions and ESLint recommended practices.

#### Scenario: Components directory exists
- **GIVEN** the frontend source structure
- **WHEN** listing directories under `web/src/`
- **THEN** a `components/` directory MUST exist
- **AND** a `component/` directory MUST NOT exist

#### Scenario: Component imports use plural path
- **GIVEN** the directory has been renamed
- **WHEN** searching for component import statements
- **THEN** all imports MUST use `@/components/` path
- **AND** NO imports MUST reference `@/component/`

### Requirement: Asset Files in Public Directory
Static asset files (images, fonts, etc.) SHALL be located in the `public/` directory, not within the `src/` directory. This ensures proper Vite handling and prevents unnecessary build processing.

#### Scenario: No images in src directory
- **GIVEN** the frontend source structure
- **WHEN** listing contents of `web/src/`
- **THEN** a `pic/` directory MUST NOT exist
- **AND** image files MUST NOT be directly in `src/`

#### Scenario: Images in public directory
- **GIVEN** static image assets need to be stored
- **WHEN** placing image files
- **THEN** they MUST be in `web/public/assets/` or `web/public/images/`
- **AND** referenced in code as `/assets/...` or `/images/...`

### Requirement: Frontend Builds Successfully
After directory restructuring, the frontend project SHALL build without errors and produce a functional distribution.

#### Scenario: Development build succeeds
- **GIVEN** the restructured frontend code
- **WHEN** running `npm run build` in web/ directory
- **THEN** the build MUST complete successfully
- **AND** a `dist/` directory MUST be created
- **AND** NO import errors MUST occur

#### Scenario: Type checking passes
- **GIVEN** TypeScript is configured
- **WHEN** running type checking
- **THEN** NO type errors related to missing modules MUST occur
- **AND** all component imports MUST resolve correctly
