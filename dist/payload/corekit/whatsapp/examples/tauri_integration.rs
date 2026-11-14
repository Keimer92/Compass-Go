// ============================================================================
//  INTEGRACIÓN TAURI (Rust) — WhatsApp Engine centralizado
//  ----------------------------------------------------------------------------
//  El motor canónico vive en: D:\Mis Proyectos\WhatsApp Engine
//  Empaquetar para Tauri:  node bundle.mjs --asset   (genera carpeta con assets)
//  Copiar la carpeta generada a:  src-tauri/resources/whatsapp-engine-bundle/
//  Este snippet asume que el bundle ya está en resources y usa tauri-plugin-shell.
// ============================================================================

use tauri::{AppHandle, Manager, Emitter};
use tauri_plugin_shell::ShellExt;
use tauri_plugin_shell::process::{CommandEvent, CommandChild};
use tauri::path::BaseDirectory;
use serde::Serialize;
use std::sync::{Arc, Mutex};

#[derive(Debug, Serialize, Clone)]
pub struct WhatsappStatus {
    pub status: String,
    pub qr_data_url: Option<String>,
    pub message: Option<String>,
    pub client_info: Option<String>,
}

lazy_static::lazy_static! {
    static ref WA_PROCESS: Arc<Mutex<Option<CommandChild>>> = Arc::new(Mutex::new(None));
    static ref WA_STARTING: Arc<Mutex<bool>> = Arc::new(Mutex::new(false));
    static ref WA_LAST: Arc<Mutex<WhatsappStatus>> = Arc::new(Mutex::new(WhatsappStatus {
        status: "disconnected".to_string(),
        qr_data_url: None,
        message: Some("Motor no iniciado".to_string()),
        client_info: None,
    }));
}

#[tauri::command]
pub async fn start_whatsapp(app: AppHandle) -> Result<(), String> {
    if { *WA_STARTING.lock().unwrap() } || { WA_PROCESS.lock().unwrap().is_some() } {
        return Ok(());
    }
    *WA_STARTING.lock().unwrap() = true;

    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        // 1. Resolver bundle (relativo a la app instalada, NO al directorio de proyecto)
        let bundle_path = app
            .path()
            .resolve("resources/whatsapp-engine-bundle/index.js", BaseDirectory::Resource)
            .map_err(|e| e.to_string())
            .unwrap();
        let bundle_path = bundle_path.to_string_lossy().replace("\\\\?\\", "");

        // 2. Working dir = directorio de datos de la app (ahí se guarda whatsapp_auth)
        let app_data_dir = app.path().app_data_dir().unwrap();

        // 3. Lanzar node con el bundle
        let (mut rx, child) = app
            .shell()
            .sidecar("node")
            .unwrap()
            .args([bundle_path.as_str()])
            .current_dir(&app_data_dir)
            .spawn()
            .map_err(|e| e.to_string())
            .unwrap();

        *WA_PROCESS.lock().unwrap() = Some(child);
        *WA_STARTING.lock().unwrap() = false;

        // 4. Escuchar eventos del motor
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line) => {
                    let l = String::from_utf8_lossy(&line).trim().to_string();
                    if l.starts_with("QR_DATA_URL:") {
                        let url = l.replace("QR_DATA_URL:", "").trim().to_string();
                        app.emit("whatsapp-qr", &url).ok();
                    } else if l.starts_with("STATUS:connected:") {
                        let info = l.replace("STATUS:connected:", "").trim().to_string();
                        app.emit("whatsapp-status", &WhatsappStatus {
                            status: "connected".into(), qr_data_url: None,
                            message: Some("✓ WhatsApp Conectado".into()), client_info: Some(info),
                        }).ok();
                    } else if l.starts_with("STATUS:") {
                        let code = l.replace("STATUS:", "").trim().to_string();
                        let (st, msg) = match code.as_str() {
                            c if c.starts_with("connecting") => ("connecting", "Conectando..."),
                            c if c.starts_with("restarting") => ("connecting", "Reintentando..."),
                            c if c.starts_with("session_invalid") => ("disconnected", "Sesión inválida"),
                            c if c.starts_with("session_cleared") => ("disconnected", "Sesión limpiada"),
                            c if c.starts_with("error") => ("disconnected", "Error"),
                            c if c.starts_with("disconnected") => ("disconnected", "Desconectado"),
                            _ => ("connecting", "Cargando..."),
                        };
                        app.emit("whatsapp-status", &WhatsappStatus {
                            status: st.to_string(), qr_data_url: None,
                            message: Some(msg.to_string()), client_info: None,
                        }).ok();
                    }
                }
                CommandEvent::Terminated(_p) => {
                    *WA_PROCESS.lock().unwrap() = None;
                }
                _ => {}
            }
        }
    });

    Ok(())
}

#[tauri::command]
pub async fn send_whatsapp_direct(app: AppHandle, number: String, message: String) -> Result<(), String> {
    let clean = number.replace(['+', '-', ' '], "");
    let mut guard = WA_PROCESS.lock().unwrap();
    if let Some(child) = guard.as_mut() {
        let payload = serde_json::json!({ "type": "send", "number": clean, "text": message });
        child
            .write(format!("{}\n", payload).as_bytes())
            .map_err(|e| e.to_string())?;
    } else {
        return Err("El motor de WhatsApp no está iniciado".to_string());
    }
    Ok(())
}

#[tauri::command]
pub async fn send_whatsapp_image(app: AppHandle, number: String, image_path: String, caption: String) -> Result<(), String> {
    let mut guard = WA_PROCESS.lock().unwrap();
    if let Some(child) = guard.as_mut() {
        let payload = serde_json::json!({
            "type": "send_image", "number": number, "imagePath": image_path, "caption": caption
        });
        child
            .write(format!("{}\n", payload).as_bytes())
            .map_err(|e| e.to_string())?;
    } else {
        return Err("El motor de WhatsApp no está iniciado".to_string());
    }
    Ok(())
}

#[tauri::command]
pub async fn logout_whatsapp(app: AppHandle) -> Result<(), String> {
    if let Some(child) = WA_PROCESS.lock().unwrap().take() {
        let _ = child.kill();
    }
    let auth = app.path().app_data_dir().unwrap().join("whatsapp_auth");
    if auth.exists() { let _ = std::fs::remove_dir_all(auth); }
    Ok(())
}