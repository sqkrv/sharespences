const log_el = document.getElementById("log");

function log(...messages) {
  console.log(...messages);
  log_el.innerText +=
    "\n" + messages.map((m) => JSON.stringify(m, null, 2)).join(" ");
}

function error(message) {
  console.error(message);
  log_el.innerText += "\n" + message;
  throw Error("got error:" + message);
}

const asArrayBuffer = (v) =>
  Uint8Array.from(atob(v.replace(/_/g, "/").replace(/-/g, "+")), (c) =>
    c.charCodeAt(0)
  );
// const asArrayBuffer = (v) => {
//   // Replace URL-safe characters and add padding
//   console.log("Base64 Input:", v);
//   const base64 = v.replace(/_/g, "/").replace(/-/g, "+");
//   const paddedBase64 = base64 + "=".repeat((4 - base64.length % 4) % 4);
//
//   try {
//     return Uint8Array.from(atob(paddedBase64), (c) => c.charCodeAt(0));
//   } catch (e) {
//     throw new Error("Invalid Base64 string");
//   }
// };
const asBase64 = (ab) => btoa(String.fromCharCode(...new Uint8Array(ab)));

const base_path = `/api/v1/auth`;

// async function getPublicKey(path, element) {
//   // const user_id = document.getElementById(element).value
//   const r = await fetch(`${base_path}/${path}`, {
//     method: "POST"
//   });
//   if (r.status !== 200) {
//     error(`Unexpected response ${r.status}: ${await r.text()}`);
//   }
//   return await r.json();
// }

async function post(path, element, creds) {
  const user_id = document.getElementById(element).value;
  const { attestationObject, clientDataJSON, signature, authenticatorData } =
    creds.response;
  const data = {
    id: creds.id,
    rawId: asBase64(creds.rawId),
    response: {
      attestationObject: asBase64(attestationObject),
      clientDataJSON: asBase64(clientDataJSON),
    },
    user_id: user_id,
  };
  if (signature) {
    data.response.signature = asBase64(signature);
    data.response.authenticatorData = asBase64(authenticatorData);
  }
  const r2 = await fetch(`${base_path}/${path}`, {
    method: "POST",
    body: JSON.stringify(data),
    headers: { "content-type": "application/json" },
  });
  if (r2.status !== 200) {
    error(`Unexpected response ${r2.status}: ${await r2.text()}`);
  }
}

async function register() {
  // const publicKey = await getPublicKey("register/options", "user-id-register");
  const user_name = document.getElementById("username-register").value
  const display_name = document.getElementById("display-name-register").value
  const options_data = {
    user_name: user_name,
    display_name: display_name,
  };
  const r = await fetch(`${base_path}/register/options`, {
    method: "POST",
    body: JSON.stringify(options_data),
    headers: { "content-type": "application/json" },
  });
  if (r.status !== 200) {
    error(`Unexpected response ${r.status}: ${await r.text()}`);
  }
  const publicKey = await r.json();
  console.log("register get response:", publicKey);
  publicKey.user.id = asArrayBuffer(publicKey.user.id);
  publicKey.challenge = asArrayBuffer(publicKey.challenge);
  if (publicKey.hints === null) {
    delete publicKey.hints;
  }
  let creds;
  try {
    creds = await navigator.credentials.create({ publicKey });
  } catch (err) {
    log("refused:", err.toString());
    return;
  }
  // await post("register", "user-id-register", creds);
  // const user_id = document.getElementById(element).value;
  const { attestationObject, clientDataJSON, signature, authenticatorData } =
    creds.response;
  const data = {
    id: creds.id,
    rawId: asBase64(creds.rawId),
    response: {
      attestationObject: asBase64(attestationObject),
      clientDataJSON: asBase64(clientDataJSON),
    },
    // user_id: document.getElementById("user-id-register").value,
    user_id: asBase64(publicKey.user.id),
    username: document.getElementById("username-register").value,
    display_name: document.getElementById("display-name-register").value,
    email: document.getElementById("user-email-register").value,
  };
  if (signature) {
    data.response.signature = asBase64(signature);
    data.response.authenticatorData = asBase64(authenticatorData);
  }
  const r2 = await fetch(`${base_path}/register`, {
    method: "POST",
    body: JSON.stringify(data),
    headers: { "content-type": "application/json" },
  });
  if (r2.status !== 200) {
    error(`Unexpected response ${r2.status}: ${await r2.text()}`);
  }
  log("registration successful");
}

async function authenticate() {
  // const publicKey = await getPublicKey("auth/options", "user-id-auth");
  const r = await fetch(`${base_path}/auth/options`);
  if (r.status !== 200) {
    error(`Unexpected response ${r.status}: ${await r.text()}`);
  }
  const publicKey = await r.json();
  console.log("auth get response:", publicKey);
  publicKey.challenge = asArrayBuffer(publicKey.challenge);
  // publicKey.allowCredentials[0].id = asArrayBuffer(publicKey.allowCredentials[0].id)
  let creds;
  try {
    creds = await navigator.credentials.get({
      publicKey,
      mediation: "conditional",
    });
    console.log(creds);
  } catch (err) {
    log("refused:", err.toString());
    return;
  }
  await post("auth", "user-id-auth", creds);
  log("authentication successful");
}
