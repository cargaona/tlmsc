{ pkgs, config, ... }:

let
  repoRoot = config.devenv.root;

  # The bot shells out to `rip` and `beet` rather than linking any library, so
  # the dev shell only has to put the same two CLIs on PATH that the Docker
  # runtime installs via pip. Versions drift from the image (nixpkgs pins
  # streamrip 2.2.x, the Dockerfile takes whatever pip resolves) -- that is
  # acceptable for local work but is why `docker-build` stays the release path.
  runtimeCLIs = [
    pkgs.streamrip
    pkgs.beets
    pkgs.ffmpeg # streamrip transcodes through it
  ];

  # Local stand-ins for the container's /data/staging and /mnt/seagate/music.
  # Both are gitignored. Nothing here writes outside the repo.
  stagingDir = "${repoRoot}/.devenv-state/staging";
  musicDir = "${repoRoot}/.devenv-state/music";
  beetsDir = "${repoRoot}/.devenv-state/beets";
in
{
  # `.env` in this repo is docker-compose's, not the dev shell's -- devenv would
  # otherwise nag on every enter.
  dotenv.disableHint = true;

  packages = [
    pkgs.go
    pkgs.gopls
    pkgs.golangci-lint # `make lint` prefers it, falls back to go vet
    pkgs.gotools # goimports
  ] ++ runtimeCLIs;

  env = {
    # Mirrors internal/config/config.go. The container gets these from the
    # k8s Secret / compose env; locally they point at the state dir above so a
    # stray download never lands in / or in the real library.
    STAGING_PATH = stagingDir;
    MUSIC_LIBRARY_PATH = musicDir;
    BEETSDIR = beetsDir;
    DEBUG = "true";

    # streamrip resolves its config relative to HOME. Pointing it at the state
    # dir keeps the dev credentials out of ~/.config/streamrip, which is the
    # one the user's own `rip` uses.
    STREAMRIP_HOME = "${repoRoot}/.devenv-state/streamrip-home";
  };

  enterShell = ''
    mkdir -p "$STAGING_PATH" "$MUSIC_LIBRARY_PATH" "$BEETSDIR" "$STREAMRIP_HOME"

    # Credentials live in the gitignored .env (same file compose reads), so
    # nothing secret ends up in devenv.nix or the world-readable nix store.
    #
    # Only the credential keys are pulled across. .env also carries compose's
    # own STAGING_PATH/BEETSDIR, and a blanket `set -a; . .env` would let those
    # container paths overwrite the state dirs set above.
    if [ -f "$DEVENV_ROOT/.env" ]; then
      for _tl_key in TELEGRAM_BOT_TOKEN QOBUZ_EMAIL QOBUZ_PASSWORD DEEZER_ARL; do
        _tl_val="$(sed -n "s/^''${_tl_key}=//p" "$DEVENV_ROOT/.env" | tail -1)"
        # Strip one layer of surrounding quotes if present.
        _tl_val="''${_tl_val%\"}"; _tl_val="''${_tl_val#\"}"
        _tl_val="''${_tl_val%\'}"; _tl_val="''${_tl_val#\'}"
        [ -n "$_tl_val" ] && export "$_tl_key=$_tl_val"
      done
      unset _tl_key _tl_val
    fi

    if [ -t 1 ]; then
      echo "telemusic dev shell ready - run 'tlhelp' for commands."
    fi
  '';

  scripts = {
    tlrun = {
      description = "Build and run the bot against the local state dirs";
      exec = ''
        set -euo pipefail
        if [ -z "''${TELEGRAM_BOT_TOKEN:-}" ]; then
          echo "Error: TELEGRAM_BOT_TOKEN is unset. Put it in $DEVENV_ROOT/.env" >&2
          exit 1
        fi
        cd "$DEVENV_ROOT"
        go run ./cmd/bot
      '';
    };

    tlcheck = {
      description = "Verify the rip/beet CLIs and credentials the bot depends on";
      exec = ''
        set -uo pipefail

        echo "toolchain"
        printf '  go        %s\n' "$(go version | awk '{print $3}')"
        printf '  rip       %s\n' "$(rip --version 2>&1 | head -1)"
        printf '  beet      %s\n' "$(beet version 2>&1 | head -1)"

        echo ""
        echo "credentials (from .env)"
        # Lengths only -- never echo the values themselves.
        for v in TELEGRAM_BOT_TOKEN QOBUZ_EMAIL QOBUZ_PASSWORD DEEZER_ARL; do
          eval "val=\''${$v:-}"
          if [ -z "$val" ]; then
            printf '  %-20s MISSING\n' "$v"
          elif [ "$val" = "missing" ]; then
            printf '  %-20s PLACEHOLDER ("missing")\n' "$v"
          else
            printf '  %-20s set (%s chars)\n' "$v" "''${#val}"
          fi
        done

        echo ""
        echo "live auth (hits the provider APIs)"

        # `rip` reads its config from XDG_CONFIG_HOME, never from the env vars
        # above, so testing the *shell's* credentials means writing them into a
        # throwaway config first. Same awk patching as scripts/entrypoint.sh --
        # if these two ever diverge, this check stops reflecting production.
        _tl_home="$(mktemp -d)"
        trap 'rm -rf "$_tl_home"' EXIT
        (
          export HOME="$_tl_home" XDG_CONFIG_HOME="$_tl_home/config"
          rip config reset -y > /dev/null 2>&1 || true
          cf="$_tl_home/config/streamrip/config.toml"
          [ -f "$cf" ] || { echo "  could not generate a streamrip config"; exit 1; }

          patch() { awk -v val="$2" -v key="$1" \
            '$0 ~ "^" key " = " {print key " = \"" val "\""; next}1' "$cf" > "$cf.t" && mv "$cf.t" "$cf"; }

          patch email_or_userid "''${QOBUZ_EMAIL:-}"
          patch password_or_token "''${QOBUZ_PASSWORD:-}"
          patch use_auth_token false
          patch arl "''${DEEZER_ARL:-}"

          for src in qobuz deezer; do
            out="$(rip search "$src" album "daft punk" 2>&1)"
            case "$out" in
              *AuthenticationError*) printf '  %-8s FAILED - rejected (wrong or expired credentials)\n' "$src" ;;
              *MissingCredentialsError*) printf '  %-8s FAILED - no credentials configured\n' "$src" ;;
              *IneligibleError*) printf '  %-8s FAILED - account has no active subscription\n' "$src" ;;
              *Error*|*Traceback*) printf '  %-8s FAILED - see: rip search %s album test\n' "$src" "$src" ;;
              *) printf '  %-8s ok\n' "$src" ;;
            esac
          done
        )
      '';
    };

    tlhelp = {
      description = "Show telemusic dev shell reference";
      exec = ''
        cat <<'HELP'
        telemusic dev shell

        COMMANDS
          tlrun     Build and run the bot (needs TELEGRAM_BOT_TOKEN in .env)
          tlcheck   Check rip/beet versions, credential presence, and live auth
          tlhelp    Show this help

          make build / test / fmt / lint      still work, unchanged
          make docker-build / docker-up       the release path (pip-pinned deps)

        ENVIRONMENT (mirrors internal/config/config.go)
          STAGING_PATH         .devenv-state/staging
          MUSIC_LIBRARY_PATH   .devenv-state/music
          BEETSDIR             .devenv-state/beets
          STREAMRIP_HOME       .devenv-state/streamrip-home
          DEBUG                true

        CREDENTIALS
          Read from $DEVENV_ROOT/.env (gitignored, shared with docker compose):
            TELEGRAM_BOT_TOKEN, QOBUZ_EMAIL, QOBUZ_PASSWORD, DEEZER_ARL

        NOTES
          - All paths stay inside .devenv-state/ so local runs never touch the
            real library at /mnt/seagate/music.
          - nixpkgs streamrip/beets may lag the versions pip installs in the
            image; verify release behaviour with 'make docker-build'.
        HELP
      '';
    };
  };
}
