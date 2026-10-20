# Packages

## winget

`winget/manifests/f/formeo/fbrescue/<version>/` is the manifest for
[winget-pkgs](https://github.com/microsoft/winget-pkgs), in the same
directory layout. It installs the Windows release archive as a portable
command, `fbrescue`. The manifests validate against the official 1.6.0
schemas.

To submit a version, on Windows with
[wingetcreate](https://github.com/microsoft/winget-create):

```
wingetcreate submit --token <GitHub token> packaging\winget\manifests\f\formeo\fbrescue\0.4.0
```

or copy the directory to a fork of `microsoft/winget-pkgs` and open a pull
request. Once accepted: `winget install formeo.fbrescue`.

For a new version, `wingetcreate update` writes the new manifest from the
release:

```
wingetcreate update formeo.fbrescue --version 0.5.0 ^
  --urls https://github.com/formeo/fbrescue/releases/download/v0.5.0/fbrescue_v0.5.0_windows_amd64.zip ^
  --submit --token <GitHub token>
```

The SHA-256 in the installer manifest must match `SHA256SUMS` of the
release.
