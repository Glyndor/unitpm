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
2. It can automatically install system services (like `lynxd.service`).
3. It allows users to manage it using their native package manager (`sudo apt install ./lynx.deb` y `sudo apt remove lynx`).

---

## Step-by-Step Guide for Beginners (Windows Users)

Dado que estás usando Windows, no puedes crear un archivo `.deb` de Linux nativamente en PowerShell. ¡Pero no te preocupes! La forma más fácil de hacerlo es usando **WSL** (Windows Subsystem for Linux), que básicamente es tener un Ubuntu completo y real escondido dentro de tu Windows.

### Paso 0: Abrir y configurar WSL (Ubuntu) en Windows

1. Si nunca has usado Linux en tu PC, abre **PowerShell como Administrador** y ejecuta este comando, luego reinicia la PC:
   ```powershell
   wsl --install
   ```
2. Una vez instalado (o si ya lo tenías), abre el **Menú Inicio** de Windows.
3. Escribe **`wsl`** o **`Ubuntu`** y presiona Enter.
   *(Se abrirá una terminal negra. ¡Felicidades! Ahora estás dentro de un sistema Linux real, dentro de tu Windows).*

### Paso 1: Instalar los "Constructores" (Solo se hace una vez)

Dentro de esa terminal negra de Linux/Ubuntu, necesitas instalar los programas que saben cómo construir paquetes `.deb`.

1. Escribe esto y presiona Enter (te pedirá tu contraseña de Linux):
   ```bash
   sudo apt-get update
   ```
2. Luego escribe esto y presiona Enter (escribe `Y` si te pregunta si deseas continuar):
   ```bash
   sudo apt-get install devscripts debhelper build-essential
   ```

---

### Paso 2: Entrar a tu proyecto que está en Windows

Por increíble que parezca, desde esa terminal de Ubuntu puedes ver y entrar a todos los discos duros de tu Windows (tu disco C:, tu disco J:, etc.). Todos los discos de Windows están montados dentro de una carpeta llamada `/mnt/` en Linux.

1. Como tu proyecto de Lynx está en el disco `J:`, escribe este comando para entrar a la carpeta de tu proyecto:
   ```bash
   cd /mnt/j/Lynx
   ```
   *(Nota importante: ¡Fíjate que las diagonales ahora son hacia adelante `/` porque estamos en Linux, no en Windows!)*

---

### Paso 3: Actualizar la Versión

Cada vez que vayas a subir una versión nueva a GitHub, debes decirle al paquete `.deb` que el número de versión cambió (para que la computadora del usuario sepa que tiene que actualizarse).

1. Escribe este comando:
   ```bash
   dch -i
   ```
   *(Esto abrirá un editor de texto muy viejito en la consola. Hasta arriba, verás que ya preparó un número de versión nuevo. Simplemente baja con las flechas del teclado, escribe un mensajito corto como "New release", luego presiona `Ctrl+O` y `Enter` para guardar, y finalmente `Ctrl+X` para salir).*

---

### Paso 4: ¡Construir el Paquete!

Ahora viene la magia real. Le diremos a Linux que use la información de la carpeta `debian/` que ya tienes configurada en tu proyecto para construir el paquete.

1. Escribe este comando exacto:
   ```bash
   dpkg-buildpackage -us -uc -b
   ```

   **¿Qué significa ese comando feo?**
   - `-us -uc`: Le dice a Ubuntu "no me pidas llaves criptográficas súper avanzadas para firmar el paquete, solo constrúyelo y ya" (mantiene las cosas simples).
   - `-b`: Le dice "solo constrúyeme el archivo .deb".

2. **Espera a que termine.** Verás que la pantalla se llena de texto, está compilando tu código en Go y empacándolo.

---

### Paso 5: ¿Dónde quedó mi archivo `.deb`?

Esto es súper importante y confunde a muchos principiantes: cuando Ubuntu termina de construir el paquete `.deb`, **no lo guarda dentro de la carpeta `Lynx`**. Lo guarda un nivel atrás, es decir, **afuera de la carpeta `Lynx`**.

1. Cierra la terminal negra de Linux.
2. Abre el **Explorador de Archivos** normal de Windows.
3. Ve a tu disco **J:**.
4. Así es, búscalo justo al lado (o afuera) de tu carpeta `Lynx`, ahí en tu disco J:. Verás un archivo llamado algo como `lynx_0.0.1-1_amd64.deb`.

---

### Paso 5.5: Construir también el Ejecutable Normal (¡Súper importante!)

Como dijimos antes, hay usuarios que van a usar el paquete `.deb`, pero hay otros que simplemente quieren el programa rápido o que usan la actualización automática interna que tú programaste (`updater.go`). Para ellos, **tienes que subir también el ejecutable normal compilado**.

1. Abre una terminal normal de **PowerShell** en tu Windows (o abre una nueva pestaña en tu terminal).
2. Asegúrate de estar en la carpeta de tu proyecto (ej. `cd J:\Lynx`).
3. Ejecuta este comando exacto para decirle a Go que te construya un ejecutable para Linux (AMD64):
   ```powershell
   $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o lynx_linux_amd64 ./cmd/lynx
   ```
   *(¡Magia! Ahora verás un archivo nuevo llamado `lynx_linux_amd64` en la carpeta `Lynx` de tu proyecto).*

---

### Paso 6: Crear el Tag y el Release en GitHub

Cuando los dos archivos de arriba estén creados y brillando en tu disco duro, solo sigue la siguiente lista para sacar el Release oficial:

1. **Abre tu repositorio `Lynx`** en el navegador de GitHub.
2. En la parte derecha, busca la sección **"Releases"** y haz clic allí.
3. Haz clic arriba y a la derecha en el botón verde/gris **"Draft a new release"**.
4. Haz clic en el recuadro gris **"Choose a tag"**. Escribe la versión nueva (ej. `v0.0.1` o `v1.0.0`) y haz clic donde dice **"+ Create new tag on publish"**.
5. Ponle un título increíble a tu actualización, como "*Versión 1.0 - ¡Nuevas animaciones!*".
6. Opcionalmente, puedes escribir de qué trató este parche nuevo. Si no quieres escribir, dale clic al botón mágico **"Generate release notes"** y GitHub pondrá un resumen de tus últimos cambios automáticamente.
7. Al final donde dice grandes letras *"Attach binaries by dropping them here..."*, **Arrastra y suelta** hacia esa caja **TUS DOS ARCHIVOS**:
   - `lynx_0.0.1-1_amd64.deb` (El que buscaste en tu disco `J:` en el Paso 5).
   - `lynx_linux_amd64` (El ejecutable normal que acabas de crear en tu carpeta `J:\Lynx` en el Paso 5.5).
8. ¡Dale clic al botón verde de **"Publish release"**!

¡Terminaste! 
- Los profesionales instalarán tu sistema bajando el `.deb` y usando APT.
- El resto bajará el ejecutable y usará la actualización automática que tú construiste (`updater.go`). ¡Ambos funcionarán a la perfección!

---

### FAQ Rápido

**¿Hacer esto publica información personal mía o de mi computadora?**
**¡No, tranquilo!** Cuando construyes estos binarios utilizando Go y `dpkg-buildpackage`, tu código fuente simplemente se compila y se convierte en lenguaje máquina que la computadora entiende. Las únicas cosas que se guardan adentro del ejecutable son tu código, las librerías que usas, y tal vez rutas estáticas e internas del compilador que usa Go bajo el capó (rutas genéricas del lenguaje que son completamente seguras). Ni tus contraseñas, ni tus archivos personales, ni nada externo a tu proyecto se filtra.
