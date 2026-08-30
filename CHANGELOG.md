# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-08-30


### Added

- Initial implementation
- Add missing htmx header functions
- Add support for sse/ws & deps version bumps
- Restructured and fixed overall routing api
- Switch support to htmx 4.0 alpha
- Codegen for typesafe routes
- Result, json, oob, cache and more ([#1](https://github.com/go-thx/thx/pull/1))
- Remove validator magic ([#2](https://github.com/go-thx/thx/pull/2))
- File upload handling ([#3](https://github.com/go-thx/thx/pull/3))
- Flash messages ([#5](https://github.com/go-thx/thx/pull/5))
- Static assets routing ([#7](https://github.com/go-thx/thx/pull/7))
- Proper code comments ([#9](https://github.com/go-thx/thx/pull/9))
- Streamline public api ([#10](https://github.com/go-thx/thx/pull/10))
- Preloaded, cached built-in form decoder ([#15](https://github.com/go-thx/thx/pull/15))
- Indexed keys, maps, typed decode errors ([#18](https://github.com/go-thx/thx/pull/18))
- CachedPartial for cached partial output with TTL ([#19](https://github.com/go-thx/thx/pull/19))
- Opt-in auto OOB flash swaps ([#20](https://github.com/go-thx/thx/pull/20))
- Composable authorization rules ([#24](https://github.com/go-thx/thx/pull/24))

### Changed

- Code restructure & cleanup
- Share one context per request ([#21](https://github.com/go-thx/thx/pull/21))

### Fixed

- Query param decode too strict ([#8](https://github.com/go-thx/thx/pull/8))
- Keep full layout on htmx history restore ([#13](https://github.com/go-thx/thx/pull/13))
- Built-in form schema ([#16](https://github.com/go-thx/thx/pull/16))

