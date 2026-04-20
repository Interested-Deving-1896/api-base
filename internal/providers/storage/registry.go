// The Registry is a lookup table: name -> "how to build this provider".
//
// At startup, each backend (local, s3, r2, ...) adds itself to the registry.
// Later, when config says STORAGE_PROVIDER=s3, we ask the registry for "s3"
// and it builds an S3 provider for us.
//
// We store BUILD INSTRUCTIONS (functions), not already-built providers.
// Here's why that matters:
//
//	// If we stored built providers, we'd have to build all of them up front:
//	local := local.New(...)  // creates a folder on disk
//	s3    := s3.New(...)     // connects to AWS — but we aren't even using S3!
//	r2    := r2.New(...)     // connects to Cloudflare — also unused!
//
//	// Storing functions, we only build the ONE we actually use:
//	provider, _ := registry.Resolve("local", opts)  // only local.New runs
package storage

import "fmt"

// Factory is a function that builds a Provider from config options.
// Every backend (local, s3, r2) exposes a function with this exact shape.
//
// Example — this is what local.New looks like, and why it fits here:
//
//	func New(opts map[string]string) (storage.Provider, error) {
//	    path := opts["path"]        // reads the keys it cares about
//	    return &Local{basePath: path}, nil
//	}
//
// Why map[string]string for opts (and not a typed struct)?
// Because config values usually arrive as strings anyway — from env vars
// (STORAGE_PATH=./uploads), from CLI flags, from YAML/JSON. Using a map
// means the registry doesn't need to know that S3 wants a "region" while
// local wants a "path". Each factory grabs its own keys and parses them
// (e.g. strconv.Atoi for numbers).
type Factory func(opts map[string]string) (Provider, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register saves a factory under a name so Resolve can find it later.
//
//	reg := NewRegistry()
//	reg.Register("local", local.New)
//	reg.Register("s3", s3.New)
//
// If the same name is registered twice, the second call wins. This is
// intentional — in tests you can swap a real factory for a fake:
//
//	reg.Register("s3", func(_ map[string]string) (Provider, error) {
//	    return &fakeS3{}, nil  // no real AWS call during tests
//	})
func (r *Registry) Register(name string, f Factory) {
	r.factories[name] = f
}

// Resolve looks up the factory for a name and runs it to build a Provider.
//
//	provider, err := reg.Resolve("local", map[string]string{"path": "./uploads"})
//	// provider is now a *local.Local, ready to Upload/Download/Delete.
//
// If no factory is registered under that name (typo in config, forgot to
// import the backend's package), you get ErrProviderNotRegistered.
func (r *Registry) Resolve(name string, opts map[string]string) (Provider, error) {
	f, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotRegistered, name)
	}
	return f(opts)
}
