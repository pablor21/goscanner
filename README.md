# GoScanner

A powerful Go package for analyzing Go codebases and extracting comprehensive type information, including structs, interfaces, methods, functions, and annotations. GoScanner provides both basic type scanning and detailed analysis with support for generics, complex nested types, and annotation parsing.

## Features

- **Comprehensive Type Analysis**: Extract detailed information about structs, interfaces, enums, functions, and methods
- **Generic Type Support**: Full support for Go generics including type parameters and constraints
- **Complex Type Resolution**: Handle nested types, pointers, channels, maps, slices, and arrays
- **Comment Extraction**: Extract comments from Go source code for documentation purposes
- **Lazy Loading**: Efficient memory usage with on-demand loading of detailed type information
- **Flexible Scanning Modes**: Configure what information to extract based on your needs
- **Extensible Architecture**: Add custom processors to extend functionality

## Installation

```bash
go get github.com/pablor21/goscanner
```

## Quick Start

### Basic Usage

```go
package main

import (
    "encoding/json"
    "fmt"
    "github.com/pablor21/goscanner"
)

func main() {
    // Create a new scanner instance
    scanner := goscanner.NewScanner()
    
    // Scan the current package
    result, err := scanner.Scan()
    if err != nil {
        panic(err)
    }
    
    // Process the results
    for _, typeInfo := range result.Types {
        fmt.Printf("Found type: %s (Kind: %s)\n", 
            typeInfo.GetName(), typeInfo.GetKind())
    }
}
```

### Advanced Configuration

```go
package main

import (
    "github.com/pablor21/goscanner"
)

func main() {
    scanner := goscanner.NewScanner()
    
    // Configure scanning options
    config := goscanner.NewDefaultConfig()
    config.Mode = goscanner.ScanModeFull  // Include all information
    config.Patterns = []string{"./..."}   // Scan all packages recursively
    
    result, err := scanner.ScanWithConfig(config)
    if err != nil {
        panic(err)
    }
    
    // Access detailed information
    for _, typeInfo := range result.Types {
        // Load detailed information (lazy-loaded)
        details, err := typeInfo.Load()
        if err != nil {
            continue
        }
        
        // Access comments
        comments := typeInfo.GetComments()
        
        fmt.Printf("Type: %s\n", typeInfo.GetName())
        fmt.Printf("Comments: %v\n", comments)
    }
}
```

## Scanning Modes

GoScanner supports different scanning modes to control the level of detail extracted:

- `ScanModeNone`: No scanning
- `ScanModeTypes`: Basic type information only
- `ScanModeMethods`: Include method information
- `ScanModeFields`: Include struct field information
- `ScanModeFunctions`: Include standalone functions
- `ScanModeDocs`: Include documentation/comments

### Predefined Mode Combinations

- `ScanModeBasic`: Types + Documentation
- `ScanModeDefault`: Types + Methods + Documentation
- `ScanModeFull`: All available information

## Type Information

### Supported Type Kinds

- `struct`: Go struct types
- `interface`: Go interface types
- `function`: Standalone functions
- `method`: Struct/interface methods
- `enum`: Enumeration types
- `field`: Struct fields
- `map`: Map types
- `slice`: Slice types
- `array`: Array types
- `channel`: Channel types
- `basic`: Built-in types (string, int, bool, etc.)
- `generic`: Generic type parameters

### Type Information Structure

Each type provides:

```go
type TypeInfo interface {
    GetKind() TypeKind
    GetName() string
    GetPackage() string
    GetTypeRef() string
    IsPointer() bool
    IsGeneric() bool
    GetComments() []string      // Lazy-loaded
    Load() (*DetailedTypeInfo, error) // Load full details
}
```

## Complex Type Examples

### Generics

```go
// Generic struct with type constraints
type Container[T comparable, U any] struct {
    Key   T
    Value U
}

// Generic interface
type Repository[T any] interface {
    Save(entity T) error
    FindByID(id string) (T, error)
}
```

### Nested and Complex Types

```go
type ComplexStruct struct {
    // Nested slices and maps
    Data map[string][][]SomeType
    
    // Channels with complex types
    Updates chan *[]UpdateEvent
    
    // Anonymous struct fields
    Config struct {
        Setting1 string
        Setting2 int
    }
    
    // Pointer to anonymous struct
    Metadata *struct {
        Created time.Time
        Tags    []string
    }
}
```

## Output Format

The scanner produces structured JSON output that can be serialized:

```json
{
  "types": [
    {
      "kind": "struct",
      "name": "User",
      "package": "main",
      "typeRef": "main.User",
      "isPointer": false,
      "isGeneric": false,
      "comments": ["User represents a system user"]
    }
  ]
}
```

## Build and Development

### Prerequisites

- Go 1.25.5 or later
- Make (optional, for build automation)

### Building

```bash
# Build the project
make build

# Run tests
make test

# Development mode (uses go.mod.dev)
make dev

# Production mode (uses go.mod)
make prod
```

### Running the Example

```bash
# Navigate to the scanner directory
cd cmd/

# Run the scanner on the examples
go run main.go
```

This will generate an `output.json` file containing the analyzed type information from the examples directory.

## Examples

The `examples/` directory contains sample Go code demonstrating various language features:

- **starwars/models**: Complex struct examples with nested types
- **starwars/functions**: Function analysis examples  
- **starwars/generics**: Generic type examples
- **starwars/outofscope**: External dependency examples

## API Reference

### Core Types

- `Scanner`: Main interface for code scanning
- `TypeInfo`: Interface for type information access
- `Config`: Configuration options for scanning
- `ScanningResult`: Results of a scanning operation
- `Processor`: Interface for extending scanner functionality

### Key Functions

- `NewScanner()`: Create a new scanner instance
- `NewDefaultConfig()`: Create default configuration
- `Scan()`: Scan with default settings
- `ScanWithConfig(config)`: Scan with custom configuration

## Contributing

Contributions are welcome! Please feel free to submit issues, feature requests, or pull requests.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Dependencies

- [golang.org/x/tools](https://golang.org/x/tools): Go tools for code analysis