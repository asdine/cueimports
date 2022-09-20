# The path follows a pattern
# ./dist/BUILD-ID_TARGET/BINARY-NAME
source = ["./dist/cueimports_darwin_all/cueimports"]
bundle_id = "com.asdine.cueimports"

sign {
  application_identity = "Developer ID Application: Azeddine El Hrychy"
}

zip {
    output_path = "dist/cueimports_macos_universal.zip"
}