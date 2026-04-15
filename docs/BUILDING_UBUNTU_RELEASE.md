# Building and Releasing a .deb for Ubuntu/Debian (From Windows)

This guide explains how to package and release your application for Debian and Ubuntu distributions directly from your Windows computer.

## Do I *need* to do this?

**Short answer:** No, but it is highly recommended!

**Long answer:**
Since this project is written in Go, the Go compiler can create a single, independent executable file.
You could simply upload this executable file (like `lynx_linux_amd64`) directly to your GitHub Release. People can download it, manually give it execution permissions (`chmod +x`), and run it.

**However**, creating a `.deb` package (which is what we will do below) is the **"professional"** way to do it.
Why? Because creating a `.deb` package has huge advantages for the user:
1. It automatically places the `lynx` program in their system, ready to use from anywhere.
2. It can automatically install system services (like `lynx.lynxd.service`).
3. It allows users to manage it using their native package manager (`sudo apt install ./lynx.deb` and `sudo apt remove lynx`).

---

## Step-by-Step Guide for Beginners (Windows Users)

Since you are using Windows, you cannot create a native Linux `.deb` file directly in PowerShell. But don't worry! The easiest way to do this is by using **WSL** (Windows Subsystem for Linux), which is basically having a complete, real Ubuntu system hidden inside your Windows install.

### Step 0: Open and configure WSL (Ubuntu) on Windows

1. If you have never used Linux on your PC, open **PowerShell as Administrator** and run this command, then restart your PC:
   ```powershell
   wsl --install
   ```
2. Once installed (or if you already had it), open the Windows **Start Menu**.
3. Type **`wsl`** or **`Ubuntu`** and press Enter.
   *(A black terminal will open. Congratulations! You are now inside a real Linux system, right inside Windows).*

### Step 1: Install the "Builders" (Do this only once)

Inside that black Linux/Ubuntu terminal, you need to install the programs that know how to build `.deb` packages.

1. Type this and press Enter (it will ask for your Linux password):
   ```bash
   sudo apt-get update
   ```
2. Then type this and press Enter (type `Y` if it asks if you want to continue):
   ```bash
   sudo apt-get install devscripts debhelper build-essential
   ```

---

### Step 2: Enter your project folder from Linux

Believe it or not, from that Ubuntu terminal you can see and access all your Windows hard drives (your C: drive, your D: drive, etc.). All Windows drives are mounted inside a folder called `/mnt/` in Linux.

1. Navigate to your project folder (e.g., if it's in `C:\Users\YourName\Lynx`):
   ```bash
   cd /mnt/c/Users/YourName/Lynx
   ```
   *(Important note: Notice that the slashes are now forward slashes `/` because we are in Linux, not Windows!)*

---

### Step 3: Update the Version

Every time you are going to upload a new version to GitHub, you must tell the `.deb` package that the version number changed (so the user's computer knows it has to update).

1. Type this command:
   ```bash
   dch -i
   ```
   *(This will open a very old-school text editor in the console. At the very top, you will see it has already prepared a new version number. Just go down with the keyboard arrows, type a short message like "New release", then press `Ctrl+O` and `Enter` to save, and finally `Ctrl+X` to exit).*

---

### Step 4: Build the Package!

Now comes the real magic. We will tell Linux to use the information in the `debian/` folder (which is already configured in your project) to build the package.

1. Type this exact command:
   ```bash
   dpkg-buildpackage -us -uc -b
   ```

   **What does this ugly command mean?**
   - `-us -uc`: Tells Ubuntu "don't ask me for super advanced cryptographic keys to sign the package, just build it" (keeps things simple).
   - `-b`: Tells it "only build the .deb file for me".

2. **Wait for it to finish.** You will see the screen fill with text; it is compiling your Go code and packaging it.

---

### Step 5: Where did my `.deb` file go?

This is super important and confuses many beginners: when Ubuntu finishes building the `.deb` package, **it does not save it inside the `Lynx` folder**. It saves it one level back, that is, **outside the `Lynx` folder**.

1. Close the black Linux terminal.
2. Open the normal Windows **File Explorer**.
3. Go to the drive where your project is located.
4. Look for it right next to (or outside of) your `Lynx` folder. You will see a file named something like `lynx-pm_0.0.1-1_amd64.deb`.

---

### Step 5.5: Build the Standard Executable too (Super important!)

As we said before, some users will use the `.deb` package via APT, but others simply want the fast program or they rely on the internal automatic update you programmed (`updater.go`). For them, **you must also upload the standard compiled executable**.

1. Open a normal **PowerShell** terminal on your Windows (or open a new tab in your terminal).
2. Make sure you are in your project folder (e.g., `cd C:\Users\YourName\Lynx`).
3. Run this exact command to tell Go to build a Linux (AMD64) executable for you:
   ```powershell
   $COMMIT=$(git rev-parse --short HEAD); $DATE=$(Get-Date -UFormat +"%Y-%m-%dT%H:%M:%SZ"); $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags="-X 'github.com/Jaro-c/Lynx/internal/version.Commit=$COMMIT' -X 'github.com/Jaro-c/Lynx/internal/version.BuildDate=$DATE'" -o lynx_linux_amd64 ./cmd/lynx
   ```
   *(Magic! You will now see a new file called `lynx_linux_amd64` in your project's `Lynx` folder).*

---

### Step 6: Create the Tag and Release on GitHub

When the two files above are created and shining on your hard drive, just follow this list to push the official Release:

1. **Open your `Lynx` repository** in your GitHub browser.
2. On the right side, look for the **"Releases"** section and click there.
3. Click the green/grey **"Draft a new release"** button on the top right.
4. Click on the grey **"Choose a tag"** box. Type the new version (e.g., `v0.0.1` or `v1.0.0`) and click where it says **"+ Create new tag on publish"**.
5. Give your update an awesome title, like "*Version 1.0 - New animations!*".
6. Optionally, you can write what this new patch was about. If you don't want to type, click the magical **"Generate release notes"** button and GitHub will automatically put a summary of your latest commits.
7. At the very bottom where it says in large letters *"Attach binaries by dropping them here..."*, **Drag and drop** into that box **YOUR TWO FILES**:
   - `lynx-pm_0.0.1-1_amd64.deb` (The one you found in Step 5).
   - `lynx_linux_amd64` (The normal executable you just created in your `Lynx` folder in Step 5.5).
8. Click the green **"Publish release"** button!

You're done! 
- Professionals will install your system by downloading the `.deb` and using APT.
- The rest will download the executable and use the automatic update tool you built (`updater.go`). Both will work perfectly!

---

### Quick FAQ

**Does doing this publish my personal information or computer data?**
**No, don't worry!** When you build these binaries using Go and `dpkg-buildpackage`, your source code is simply compiled into machine language that the computer understands. The only things saved inside the executable are your code, the libraries you use, and perhaps static internal paths from the compiler that Go uses under the hood (generic language paths that are completely safe). Neither your passwords, nor your personal files, nor anything external to your project is ever leaked.
