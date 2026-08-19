{
  description = "watch-together: Rust backend, React frontend, and OCI image";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

    nix2container = {
      url = "github:nlewo/nix2container";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = {
    nixpkgs,
    nix2container,
    ...
  }: let
    systems = [
      "x86_64-linux"
      "aarch64-linux"
    ];

    forAllSystems = nixpkgs.lib.genAttrs systems;

    mkProject = system: let
      pkgs = import nixpkgs {inherit system;};
      inherit (pkgs) lib;

      n2c = nix2container.packages.${system}.nix2container;

      pname = "watch-together";
      version = "0.1.0";

      nodejs = pkgs.nodejs_22;

      imageName = "docker.io/maneeshwije/watch-together";
      imageTag = "latest";

      containerUser = "watch-together";
      containerUid = 10001;
      containerGid = 10001;

      # --------------------------------------------------
      # Python environment used by yt-dlp plugins
      # --------------------------------------------------
      python = pkgs.python3;

      pythonEnv = python.withPackages (ps: [
        ps.bgutil-ytdlp-pot-provider
      ]);

      # --------------------------------------------------
      # Sources
      # --------------------------------------------------
      backendSrc = lib.cleanSourceWith {
        src = ./backend;

        filter = path: type: let
          name = baseNameOf path;
        in
          lib.cleanSourceFilter path type
          && name != "target"
          && !(lib.hasPrefix ".env" name);
      };

      frontendSrc = lib.cleanSourceWith {
        src = ./frontend;

        filter = path: type: let
          name = baseNameOf path;
        in
          lib.cleanSourceFilter path type
          && name != "node_modules"
          && name != "dist"
          && !(lib.hasPrefix ".env" name);
      };

      # --------------------------------------------------
      # Rust backend
      # --------------------------------------------------
      backend = pkgs.rustPlatform.buildRustPackage {
        inherit pname version;

        src = backendSrc;

        cargoLock.lockFile = ./backend/Cargo.lock;

        SQLX_OFFLINE = "true";
        OPENSSL_NO_VENDOR = "1";

        nativeBuildInputs = [
          pkgs.pkg-config
        ];

        buildInputs = [
          pkgs.openssl
        ];

        doCheck = false;

        meta.mainProgram = "watch-together";
      };

      # --------------------------------------------------
      # React frontend
      # --------------------------------------------------
      frontend = pkgs.buildNpmPackage {
        pname = "${pname}-frontend";
        inherit version nodejs;

        src = frontendSrc;

        npmDeps = pkgs.importNpmLock {
          npmRoot = frontendSrc;
        };

        npmConfigHook = pkgs.importNpmLock.npmConfigHook;

        npmBuildScript = "build";

        installPhase = ''
          runHook preInstall

          mkdir -p "$out"
          cp -R dist/. "$out/"

          runHook postInstall
        '';
      };

      # --------------------------------------------------
      # Application root
      # --------------------------------------------------
      appRoot = pkgs.runCommand "${pname}-image-root" {} ''
        install -Dm755 \
          ${backend}/bin/watch-together \
          "$out/app/watch-together"

        mkdir -p "$out/app/dist"
        mkdir -p "$out/tmp"

        cp -R ${frontend}/. "$out/app/dist/"
      '';

      # --------------------------------------------------
      # /etc/passwd + /etc/group
      # --------------------------------------------------
      userRoot = pkgs.dockerTools.fakeNss.override {
        extraPasswdLines = [
          "${containerUser}:x:${toString containerUid}:${toString containerGid}:watch-together:/app:/sbin/nologin"
        ];

        extraGroupLines = [
          "${containerUser}:x:${toString containerGid}:"
        ];
      };

      # --------------------------------------------------
      # Runtime
      #
      # Important:
      # pythonEnv contains bgutil-ytdlp-pot-provider.
      # --------------------------------------------------
      runtimeRoot = pkgs.buildEnv {
        name = "${pname}-runtime-root";

        paths = [
          pkgs.cacert

          # yt-dlp itself
          pkgs.yt-dlp

          # Python + bgutil yt-dlp plugin
          pythonEnv

          # JS challenge runtime
          pkgs.deno

          # Video processing
          pkgs.ffmpeg-headless
        ];

        ignoreCollisions = true;

        pathsToLink = [
          "/bin"
          "/lib"
          "/etc/ssl/certs"
        ];
      };

      # --------------------------------------------------
      # OCI container
      # --------------------------------------------------
      container = n2c.buildImage {
        name = imageName;
        tag = imageTag;

        copyToRoot = [
          runtimeRoot
          appRoot
          userRoot
        ];

        maxLayers = 100;

        perms = [
          {
            path = appRoot;
            regex = "/app";
            mode = "0755";
            uid = containerUid;
            gid = containerGid;
            uname = containerUser;
            gname = containerUser;
          }

          {
            path = appRoot;
            regex = "/tmp";
            mode = "1777";
          }
        ];

        config = {
          Entrypoint = [
            "/app/watch-together"
          ];

          WorkingDir = "/app";

          User = containerUser;

          Env = [
            "PATH=/bin"

            "HOME=/tmp"
            "TMPDIR=/tmp"

            "LANG=C.UTF-8"
            "LC_ALL=C.UTF-8"

            "RUST_LOG=info"

            "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
            "SSL_CERT_DIR=/etc/ssl/certs"

            # This is what makes the bgutil yt-dlp plugin
            # discoverable by yt-dlp.
            "PYTHONPATH=${pythonEnv}/${python.sitePackages}"
          ];

          ExposedPorts = {
            "8080/tcp" = {};
          };

          Labels = {
            "org.opencontainers.image.title" = pname;
            "org.opencontainers.image.version" = version;

            "org.opencontainers.image.source" = "https://github.com/ManeeshWije/watch-together";
          };
        };
      };

      # --------------------------------------------------
      # Development shell
      # --------------------------------------------------
      devShell = pkgs.mkShell {
        packages = [
          # Rust
          pkgs.cargo
          pkgs.rustc
          pkgs.rust-analyzer
          pkgs.rustfmt
          pkgs.clippy
          pkgs.bacon
          pkgs.sqlx-cli

          pkgs.pkg-config
          pkgs.cmake
          pkgs.ninja
          pkgs.clang
          pkgs.llvmPackages.libclang
          pkgs.perl

          # Frontend
          nodejs

          # Runtime / infra
          pkgs.postgresql
          pkgs.yt-dlp
          pythonEnv
          pkgs.deno
          pkgs.ffmpeg-headless

          pkgs.skopeo
          pkgs.curl
          pkgs.jq
          pkgs.git
        ];

        buildInputs = [
          pkgs.openssl
        ];

        SQLX_OFFLINE = "true";
        OPENSSL_NO_VENDOR = "1";

        LIBCLANG_PATH = "${pkgs.llvmPackages.libclang.lib}/lib";

        # Makes testing yt-dlp locally use the same plugin.
        PYTHONPATH = "${pythonEnv}/${python.sitePackages}";
      };
    in {
      inherit
        backend
        frontend
        container
        devShell
        ;
    };
  in {
    # ------------------------------------------------------
    # Packages
    # ------------------------------------------------------
    packages = forAllSystems (
      system: let
        project = mkProject system;
      in {
        inherit
          (project)
          backend
          frontend
          container
          ;

        default = project.container;
      }
    );

    # ------------------------------------------------------
    # Checks
    # ------------------------------------------------------
    checks = forAllSystems (
      system: let
        project = mkProject system;
      in {
        inherit
          (project)
          backend
          frontend
          ;
      }
    );

    # ------------------------------------------------------
    # Dev shells
    # ------------------------------------------------------
    devShells = forAllSystems (
      system: {
        default = (mkProject system).devShell;
      }
    );

    # ------------------------------------------------------
    # Image helper apps
    # ------------------------------------------------------
    apps = forAllSystems (
      system: let
        image = (mkProject system).container;
      in {
        load = {
          type = "app";

          program = "${image.copyToDockerDaemon}/bin/copy-to-docker-daemon";

          meta.description = "Load the watch-together image into Docker";
        };

        push = {
          type = "app";

          program = "${image.copyToRegistry}/bin/copy-to-registry";

          meta.description = "Push the watch-together image to Docker Hub";
        };

        load-podman = {
          type = "app";

          program = "${image.copyToPodman}/bin/copy-to-podman";

          meta.description = "Load the watch-together image into Podman";
        };
      }
    );
  };
}
