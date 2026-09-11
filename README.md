Go-Core Template App
================================

> [Aeon Digital](http://aeondigital.com.br)  
> rianna@aeondigital.com.br

&nbsp;

> Official repository template blueprint designed to instant-bootstrap production-ready Go applications with built-in centralized governance, dynamic architecture injection, and automated cloud CI/CD pipelines.




&nbsp;
________________________________________________________________________________

## 1. PURPOSE & BOOTSTRAPPING LIFECYCLE

This repository serves as the universal gateway for initializing any new software
project within the Go-Core family ecosystem.

Instead of manually duplicating workflows, build strategies, or local git hooks,
developers use this template to spawn a clean repository that instantly inherits
our entire engineering standards infrastructure.




&nbsp;
________________________________________________________________________________

## 2. HOW TO SPAWN A NEW REPOSITORY

To create a new project using this architectural intelligence, execute the following
onboarding sequence



&nbsp;
---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- 

### 2.1 Step 1: Trigger GitHub Template Generation

- Navigate to the main page of `Go-Core-Template-App` on GitHub.
- Click the green **"Use this template"** button located at the top right.
- Select **"Create a new repository"**.
- Fill in your new repository name and visibility parameters, then click confirm.



&nbsp;
---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- 

### 2.2 Step 2: Clone Your New Local Workspace

Clone your newly generated repository onto your local workstation workstation and
move into its root folder:

```bash
git clone https://github.com/<your_user>/<project_name>.git
cd project_name
```




&nbsp;
________________________________________________________________________________

## 3. INITIALIZING THE ECOSYSTEM BLUEPRINT

When a repository is spawned from a GitHub template, it arrives as a raw file copy
without internal dependencies, local configurations, or activated gates.

To complete the setup, you must run the interactive onboarding automation script.



&nbsp;
---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- 

### 3.1 Executing the Onboarding Wizard

Run the initialization script from the root level of your workspace:

```bash
./init.sh
```



&nbsp;
---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- 

### 3.2 What the Script Accomplishes Automagically

Once evoked, the wizard will hold terminal execution to request critical metadata
constraints via an interactive prompt, executing the following operations behind
the scenes:

- **Submodule Provisioning:** Mounts the central governance hub (`Go-Core-Template-Dev`)
  natively inside your hidden `.dev/` technical folder.
- **Metadata Hydration:** Populates your project metadata database (`.github/config.txt`)
  with your principal package shorthand prefix.
- **Go Module Generation:** Dynamically initializes your `go.mod` using your exact
  custom domain path.
- **Automated Gate Activation:** Redirects your local execution path to the centralized
  Git lifecycle hooks (`pre-commit` and `pre-push`).
- **Self-Cleanup Process:** Safely destroys the temporary `init.sh` file to leave
  your directory layout pristine.
- **Genesis Commit:** Stages all mutated configuration structures and fires your
  very first local git commit.