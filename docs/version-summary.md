# Agent Infrastructure - Version Summary

## Local Environment
- **Go**: 1.26.1
- **Node.js**: v18.19.1
- **npm**: 9.2.0
- **pnpm**: Not installed (will use npm or yarn)

## Backend Dependencies (Go)

| Package | Version | Notes |
|---------|---------|-------|
| go | 1.26 | Matches local Go 1.26.1 |
| gin-gonic/gin | v1.12.0 | Latest stable, requires Go 1.25+ |
| gorm.io/gorm | v1.31.1 | Latest stable ORM |
| gorm.io/driver/postgres | v1.5.9 | PostgreSQL driver for GORM |
| google/uuid | v1.6.0 | UUID generation |
| sashabaranov/go-openai | v1.38.0 | Community OpenAI client |
| qdrant/go-client | v1.17.1 | Official Qdrant Go client |
| gopkg.in/yaml.v3 | v3.0.1 | YAML parsing |

## Frontend Dependencies (TypeScript/React)

| Package | Version | Notes |
|---------|---------|-------|
| react | ^19.0.0 | Latest React 19 |
| react-dom | ^19.0.0 | React DOM |
| antd | ^6.3.3 | Ant Design 6.x |
| @ant-design/x | ^2.4.0 | Ant Design X 2.x for AI UI |
| typescript | ^5.7.0 | Latest TypeScript |
| vite | ^6.0.0 | Vite 6.x |
| @storybook/react-vite | ^8.6.0 | Storybook for React |

## Storage

| Component | Version | Notes |
|-----------|---------|-------|
| PostgreSQL | 15+ | Primary database for sessions |
| Qdrant | 1.17.x | Vector database for memory |

## Key Breaking Changes Noted

### Gin v1.12.0
- Requires Go 1.25+
- Performance improvements
- Enhanced middleware support

### Ant Design 6.0
- Semantic structure support
- React 18+ required
- Breaking changes from v5 (upgrade guide available)

### Ant Design X 2.0
- Major rewrite from v1
- New streaming markdown renderer
- Built-in OpenAI SDK integration
- Improved component architecture

### GORM v1.31.x
- Better JSON support
- Enhanced plugin system
- Performance improvements