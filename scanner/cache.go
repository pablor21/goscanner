package scanner

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gstypes "github.com/pablor21/goscanner/types"
)

// CacheHeader contains metadata about the cache file
type CacheHeader struct {
	Magic     string `json:"magic"`
	Version   uint8  `json:"version"`
	Timestamp int64  `json:"timestamp"`
	Checksum  uint32 `json:"checksum"`
}

// CacheFile is a gzip-compressed JSON file containing the serialized scanning result
type CacheFile struct {
	Header CacheHeader            `json:"header"`
	Result map[string]interface{} `json:"result"` // The complete result from ScanningResult.Serialize()
}

const (
	CacheMagic   = "GSCAN"
	CacheVersion = 1
)

// WriteCache writes the scanning result to a gzip-compressed JSON cache file
func WriteCache(filename string, result *ScanningResult) error {
	if filename == "" {
		return fmt.Errorf("cache filename cannot be empty")
	}

	if result == nil {
		return fmt.Errorf("scanning result cannot be nil")
	}

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory %s: %w", dir, err)
		}
	}

	// Create cache file structure
	cache := &CacheFile{
		Header: CacheHeader{
			Magic:     CacheMagic,
			Version:   CacheVersion,
			Timestamp: time.Now().Unix(),
		},
	}

	// Serialize the result
	if serialized, ok := result.Serialize().(map[string]interface{}); ok {
		cache.Result = serialized
	} else {
		return fmt.Errorf("unexpected serialization format")
	}

	// Calculate checksum on the result data
	resultBytes, err := json.Marshal(cache.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	cache.Header.Checksum = calculateChecksum(resultBytes)

	// Marshal cache to JSON
	cacheJSON, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create cache file %s: %w", filename, err)
	}
	defer func() { _ = file.Close() }()

	// Wrap with gzip compression
	gzipWriter := gzip.NewWriter(file)
	defer func() { _ = gzipWriter.Close() }()

	// Write compressed JSON
	if _, err := gzipWriter.Write(cacheJSON); err != nil {
		_ = os.Remove(filename)
		return fmt.Errorf("failed to write compressed cache: %w", err)
	}

	return nil
}

// ReadCache reads a scanning result from a gzip-compressed JSON cache file
func ReadCache(filename string) (*ScanningResult, error) {
	if filename == "" {
		return nil, fmt.Errorf("cache filename cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, fmt.Errorf("cache file not found: %s", filename)
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat cache file: %w", err)
	}

	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache file %s: %w", filename, err)
	}
	defer func() { _ = file.Close() }()

	// Decompress gzip
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	// Decode JSON
	var cache CacheFile
	decoder := json.NewDecoder(gzipReader)
	if err := decoder.Decode(&cache); err != nil {
		return nil, fmt.Errorf("failed to decode cache: %w", err)
	}

	// Validate cache
	if cache.Header.Magic != CacheMagic {
		return nil, fmt.Errorf("invalid cache magic: expected %s, got %s", CacheMagic, cache.Header.Magic)
	}

	if cache.Header.Version != CacheVersion {
		return nil, fmt.Errorf("incompatible cache version: expected %d, got %d", CacheVersion, cache.Header.Version)
	}

	// Reconstruct ScanningResult from JSON data
	return reconstructFromCache(cache.Result)
}

// reconstructFromCache rebuilds a ScanningResult from the cached JSON data
// This properly deserializes the full type information including fields, methods, parameters, etc.
func reconstructFromCache(data map[string]interface{}) (*ScanningResult, error) {
	result := NewScanningResult()

	// First pass: reconstruct all types with basic information
	if typesData, ok := data["types"].(map[string]interface{}); ok {
		for id, typeData := range typesData {
			if typeBytes, err := json.Marshal(typeData); err == nil {
				t, err := deserializeType(string(typeBytes), result)
				if err == nil && t != nil {
					result.Types.Set(id, t)
				}
			}
		}
	}

	// Reconstruct values
	if valuesData, ok := data["values"].(map[string]interface{}); ok {
		for id, valueData := range valuesData {
			if valueBytes, err := json.Marshal(valueData); err == nil {
				v, err := deserializeValue(string(valueBytes), result)
				if err == nil && v != nil {
					result.Values.Set(id, v)
				}
			}
		}
	}

	// Reconstruct packages
	if packagesData, ok := data["packages"].(map[string]interface{}); ok {
		for path, pkgData := range packagesData {
			if pkgBytes, err := json.Marshal(pkgData); err == nil {
				p, err := deserializePackage(string(pkgBytes), result)
				if err == nil && p != nil {
					result.Packages.Set(path, p)
				}
			}
		}
	}

	return result, nil
}

// deserializeType reconstructs a Type from JSON bytes
func deserializeType(jsonStr string, result *ScanningResult) (gstypes.Type, error) {
	var st gstypes.SerializedType
	if err := json.Unmarshal([]byte(jsonStr), &st); err != nil {
		return nil, err
	}

	var t gstypes.Type

	switch st.Kind {
	case gstypes.TypeKindBasic:
		var sb gstypes.SerializedBasic
		_ = json.Unmarshal([]byte(jsonStr), &sb)
		t = gstypes.NewBasic(sb.ID, sb.Name)
		if sb.Underlying != nil {
			if underlyingType := reconstructTypeRef(sb.Underlying, result); underlyingType != nil {
				t.(*gstypes.Basic).SetUnderlying(underlyingType)
			}
		}

	case gstypes.TypeKindPointer:
		var sp gstypes.SerializedPointer
		_ = json.Unmarshal([]byte(jsonStr), &sp)
		elem := reconstructTypeRef(sp.Element, result)
		t = gstypes.NewPointer(sp.ID, sp.Name, elem, sp.Depth)

	case gstypes.TypeKindSlice:
		var ss gstypes.SerializedSlice
		_ = json.Unmarshal([]byte(jsonStr), &ss)
		elem := reconstructTypeRef(ss.Element, result)
		t = gstypes.NewSlice(ss.ID, ss.Name, elem)

	case gstypes.TypeKindArray:
		var sa gstypes.SerializedSlice
		_ = json.Unmarshal([]byte(jsonStr), &sa)
		elem := reconstructTypeRef(sa.Element, result)
		t = gstypes.NewSlice(sa.ID, sa.Name, elem)

	case gstypes.TypeKindMap:
		var sm gstypes.SerializedMap
		_ = json.Unmarshal([]byte(jsonStr), &sm)
		key := reconstructTypeRef(sm.Key, result)
		val := reconstructTypeRef(sm.Value, result)
		t = gstypes.NewMap(sm.ID, sm.Name, key, val)

	case gstypes.TypeKindChan:
		var sc gstypes.SerializedChan
		_ = json.Unmarshal([]byte(jsonStr), &sc)
		elem := reconstructTypeRef(sc.Element, result)
		t = gstypes.NewChan(sc.ID, sc.Name, elem, sc.Direction)

	case gstypes.TypeKindAlias:
		var sa gstypes.SerializedAlias
		_ = json.Unmarshal([]byte(jsonStr), &sa)
		underlying := reconstructTypeRef(sa.Underlying, result)
		t = gstypes.NewAlias(sa.ID, sa.Name, underlying)

	case gstypes.TypeKindFunction:
		var sf gstypes.SerializedFunction
		_ = json.Unmarshal([]byte(jsonStr), &sf)
		fn := gstypes.NewFunction(sf.ID, sf.Name)
		// Add parameters
		for _, param := range sf.Parameters {
			paramType := reconstructTypeRef(param.Type, result)
			fn.AddParameter(gstypes.NewParameter(param.Name, paramType, param.IsVariadic))
		}
		// Add results
		for _, res := range sf.Results {
			resType := reconstructTypeRef(res.Type, result)
			fn.AddResult(gstypes.NewResult(res.Name, resType))
		}
		t = fn

	case gstypes.TypeKindInterface:
		var si gstypes.SerializedInterface
		_ = json.Unmarshal([]byte(jsonStr), &si)
		iface := gstypes.NewInterface(si.ID, si.Name)
		// Add embeds
		for _, embed := range si.Embeds {
			if embedType := reconstructTypeRef(embed, result); embedType != nil {
				iface.AddEmbed(embedType)
			}
		}
		// Add methods
		methods := make([]*gstypes.Method, 0)
		for _, method := range si.Methods {
			m := gstypes.NewMethod(method.ID, method.Name, iface, false)
			// Add parameters
			for _, param := range method.Parameters {
				paramType := reconstructTypeRef(param.Type, result)
				m.AddParameter(gstypes.NewParameter(param.Name, paramType, param.IsVariadic))
			}
			// Add results
			for _, res := range method.Results {
				resType := reconstructTypeRef(res.Type, result)
				m.AddResult(gstypes.NewResult(res.Name, resType))
			}
			m.SetExported(method.Exported)
			methods = append(methods, m)
		}
		iface.AddMethods(methods...)
		t = iface

	case gstypes.TypeKindStruct:
		var ss gstypes.SerializedStruct
		_ = json.Unmarshal([]byte(jsonStr), &ss)
		str := gstypes.NewStruct(ss.ID, ss.Name)
		// Add embeds
		for _, embed := range ss.Embeds {
			if embedType := reconstructTypeRef(embed, result); embedType != nil {
				str.AddEmbed(embedType)
			}
		}
		// Add fields
		for _, field := range ss.Fields {
			fieldType := reconstructTypeRef(field.Type, result)
			f := gstypes.NewField(field.ID, field.Name, fieldType, field.Tag, field.IsEmbedded, str)
			str.AddField(f)
		}
		// Add methods
		methods := make([]*gstypes.Method, 0)
		for _, method := range ss.Methods {
			m := gstypes.NewMethod(method.ID, method.Name, str, method.IsPointerReceiver)
			// Add parameters
			for _, param := range method.Parameters {
				paramType := reconstructTypeRef(param.Type, result)
				m.AddParameter(gstypes.NewParameter(param.Name, paramType, param.IsVariadic))
			}
			// Add results
			for _, res := range method.Results {
				resType := reconstructTypeRef(res.Type, result)
				m.AddResult(gstypes.NewResult(res.Name, resType))
			}
			m.SetExported(method.Exported)
			methods = append(methods, m)
		}
		str.AddMethods(methods...)
		t = str

	case gstypes.TypeKindMethod:
		var sm gstypes.SerializedMethod
		_ = json.Unmarshal([]byte(jsonStr), &sm)
		receiver := reconstructTypeRef(sm.Receiver, result)
		m := gstypes.NewMethod(sm.ID, sm.Name, receiver, sm.IsPointerReceiver)
		// Add parameters
		for _, param := range sm.Parameters {
			paramType := reconstructTypeRef(param.Type, result)
			m.AddParameter(gstypes.NewParameter(param.Name, paramType, param.IsVariadic))
		}
		// Add results
		for _, res := range sm.Results {
			resType := reconstructTypeRef(res.Type, result)
			m.AddResult(gstypes.NewResult(res.Name, resType))
		}
		m.SetExported(sm.Exported)
		t = m

	case gstypes.TypeKindField:
		var sf gstypes.SerializedField
		_ = json.Unmarshal([]byte(jsonStr), &sf)
		parent := reconstructTypeRef(sf.Parent, result)
		fieldType := reconstructTypeRef(sf.Type, result)
		f := gstypes.NewField(sf.ID, sf.Name, fieldType, sf.Tag, sf.IsEmbedded, parent)
		f.SetExported(sf.Exported)
		t = f

	case gstypes.TypeKindInstantiated:
		var sig gstypes.SerializedInstantiatedGeneric
		_ = json.Unmarshal([]byte(jsonStr), &sig)
		origin := reconstructTypeRef(sig.Origin, result)
		// Create instantiated generic type with type arguments
		typeArgs := make([]gstypes.TypeArgument, 0)
		for _, arg := range sig.TypeArgs {
			argType := reconstructTypeRef(arg, result)
			if argType != nil {
				typeArgs = append(typeArgs, gstypes.TypeArgument{
					Type: argType,
				})
			}
		}
		t = gstypes.NewInstantiatedGeneric(sig.ID, sig.Name, origin, typeArgs)

	case gstypes.TypeKindTypeParameter:
		var stp gstypes.SerializedTypeParameter
		_ = json.Unmarshal([]byte(jsonStr), &stp)
		constraint := reconstructTypeRef(stp.Constraint, result)
		t = gstypes.NewTypeParameter(stp.ID, stp.Name, stp.Index, constraint)

	case gstypes.TypeKindUnion:
		var su gstypes.SerializedUnion
		_ = json.Unmarshal([]byte(jsonStr), &su)
		// Create union terms
		terms := make([]*gstypes.UnionTerm, 0)
		for _, term := range su.Terms {
			termType := reconstructTypeRef(term.Type, result)
			if termType != nil {
				terms = append(terms, gstypes.NewUnionTerm(termType, term.Approximation))
			}
		}
		t = gstypes.NewUnion(su.ID, su.Name, terms)

	case gstypes.TypeKindConstant, gstypes.TypeKindVariable:
		// Constants and Variables are stored in the Values map, not Types
		// If we encounter them in Types, create a basic placeholder
		t = gstypes.NewBasic(st.ID, st.Name)

	default:
		// Unknown type kind
		return nil, fmt.Errorf("unknown type kind: %s", st.Kind)
	}

	if t != nil {
		// Set common fields
		t.SetExported(st.Exported)
		t.SetDistance(st.Distance)
		// Note: comments are not restored from cache to reduce cache size
	}

	return t, nil
}

// reconstructTypeRef reconstructs a type reference from serialized data
func reconstructTypeRef(data interface{}, result *ScanningResult) gstypes.Type {
	if data == nil {
		return nil
	}

	// If it's a map with "id" field, it's a reference
	if typeMap, ok := data.(map[string]interface{}); ok {
		if id, hasID := typeMap["id"].(string); hasID {
			// Check if we already have this type
			if t, exists := result.Types.Get(id); exists {
				return t
			}
			// Check if this has detailed type info we need to deserialize
			if jsonBytes, err := json.Marshal(typeMap); err == nil {
				if t, err := deserializeType(string(jsonBytes), result); err == nil && t != nil {
					return t
				}
			}
			// Fallback to placeholder
			return gstypes.NewBasic(id, id)
		}
	}

	// If it's a string, it's a type ID reference
	if id, ok := data.(string); ok {
		if t, exists := result.Types.Get(id); exists {
			return t
		}
		// Return a placeholder basic type if not found yet
		return gstypes.NewBasic(id, id)
	}

	return nil
}

// deserializeValue reconstructs a Value from JSON bytes
func deserializeValue(jsonStr string, result *ScanningResult) (*gstypes.Value, error) {
	var sv gstypes.SerializedValue
	if err := json.Unmarshal([]byte(jsonStr), &sv); err != nil {
		return nil, err
	}

	valueType := reconstructTypeRef(sv.ValueType, result)
	v := gstypes.NewVariable(sv.ID, sv.Name, valueType)
	v.SetExported(sv.Exported)

	return v, nil
}

// deserializePackage reconstructs a Package from JSON bytes
func deserializePackage(jsonStr string, result *ScanningResult) (*gstypes.Package, error) {
	var pkgData struct {
		Path string   `json:"path"`
		Name string   `json:"name"`
		Doc  string   `json:"doc,omitempty"`
		Docs []string `json:"docs,omitempty"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &pkgData); err != nil {
		return nil, err
	}

	pkg := gstypes.NewPackage(pkgData.Path, pkgData.Name, nil)
	return pkg, nil
}

// IsCacheValid checks if a cache file exists and is a valid cache file
func IsCacheValid(filename string) bool {
	if filename == "" {
		return false
	}

	info, err := os.Stat(filename)
	if os.IsNotExist(err) || err != nil {
		return false
	}

	if info.IsDir() || info.Size() == 0 {
		return false
	}

	// Try to actually validate the cache format
	_, err = ReadCache(filename)
	return err == nil
}

// CacheAge returns the age of a cache file in seconds
// Returns -1 if the file doesn't exist
func CacheAge(filename string) int64 {
	if filename == "" {
		return -1
	}

	info, err := os.Stat(filename)
	if os.IsNotExist(err) || err != nil {
		return -1
	}

	return time.Now().Unix() - info.ModTime().Unix()
}

// InvalidateCache removes a cache file
func InvalidateCache(filename string) error {
	if filename == "" {
		return fmt.Errorf("cache filename cannot be empty")
	}

	if !IsCacheValid(filename) {
		return nil // Already doesn't exist
	}

	return os.Remove(filename)
}

// ShouldUseCache determines if cached results should be used
func ShouldUseCache(cacheFile string, maxAgeSeconds int64, sourceFiles ...string) bool {
	if !IsCacheValid(cacheFile) {
		return false
	}

	// Check cache age if specified
	if maxAgeSeconds > 0 {
		age := CacheAge(cacheFile)
		if age < 0 || age > maxAgeSeconds {
			return false
		}
	}

	// Check if any source files are newer than cache
	cacheInfo, err := os.Stat(cacheFile)
	if err != nil {
		return false
	}

	cacheModTime := cacheInfo.ModTime()

	for _, sourceFile := range sourceFiles {
		sourceInfo, err := os.Stat(sourceFile)
		if err != nil {
			return false
		}

		if sourceInfo.ModTime().After(cacheModTime) {
			return false
		}
	}

	return true
}

// calculateChecksum calculates a simple checksum for data validation
func calculateChecksum(data []byte) uint32 {
	var checksum uint32
	for _, b := range data {
		checksum = ((checksum << 1) | (checksum >> 31)) ^ uint32(b)
	}
	return checksum
}
