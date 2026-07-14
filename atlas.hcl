env "runtime" {
  url = getenv("ATLAS_DATABASE_URL")

  migration {
    dir          = "file://internal/persistence/migrations"
    lock_timeout = "10s"
  }
}
