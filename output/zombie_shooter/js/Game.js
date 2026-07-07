const Game = {
  scene: null, camera: null, renderer: null, clock: null, canvas: null,
  keys: {}, mouse: { x: 0, y: 0, worldX: 0, worldZ: 0, down: false },
  player: null, zombies: [], projectiles: [], particles: [], grenades: [],
  weaponSystem: null, skillSystem: null, waveSystem: null, ui: null, particleSystem: null,
  score: 0, wave: 0, isRunning: false, isGameOver: false, timeScale: 1.0,
  WORLD_SIZE: 50,
  init() {
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x1a1a2e);
    this.scene.fog = new THREE.Fog(0x1a1a2e, 30, 80);
    this.camera = new THREE.PerspectiveCamera(50, window.innerWidth / window.innerHeight, 0.1, 200);
    this.camera.position.set(0, 35, 25);
    this.camera.lookAt(0, 0, 0);
    this.renderer = new THREE.WebGLRenderer({ antialias: true });
    this.renderer.setSize(window.innerWidth, window.innerHeight);
    this.renderer.setPixelRatio(window.devicePixelRatio);
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    document.getElementById('gameContainer').appendChild(this.renderer.domElement);
    this.canvas = this.renderer.domElement;
    this.clock = new THREE.Clock();
    const ambient = new THREE.AmbientLight(0x404060, 0.6);
    this.scene.add(ambient);
    const dir = new THREE.DirectionalLight(0xffffff, 0.8);
    dir.position.set(20, 40, 20);
    dir.castShadow = true;
    dir.shadow.camera.left = -50; dir.shadow.camera.right = 50;
    dir.shadow.camera.top = 50; dir.shadow.camera.bottom = -50;
    dir.shadow.mapSize.set(2048, 2048);
    this.scene.add(dir);
    const groundGeo = new THREE.PlaneGeometry(this.WORLD_SIZE * 2, this.WORLD_SIZE * 2);
    const groundMat = new THREE.MeshStandardMaterial({ color: 0x2a2a3e, roughness: 0.9 });
    const ground = new THREE.Mesh(groundGeo, groundMat);
    ground.rotation.x = -Math.PI / 2;
    ground.receiveShadow = true;
    this.scene.add(ground);
    const grid = new THREE.GridHelper(this.WORLD_SIZE * 2, 40, 0x333355, 0x222244);
    this.scene.add(grid);
    const wallMat = new THREE.MeshStandardMaterial({ color: 0x444466, roughness: 0.8 });
    [[0, this.WORLD_SIZE, this.WORLD_SIZE * 2, 1], [0, -this.WORLD_SIZE, this.WORLD_SIZE * 2, 1], [this.WORLD_SIZE, 0, 1, this.WORLD_SIZE * 2], [-this.WORLD_SIZE, 0, 1, this.WORLD_SIZE * 2]].forEach(w => {
      const wall = new THREE.Mesh(new THREE.BoxGeometry(w[2], 2, w[3]), wallMat);
      wall.position.set(w[0], 1, w[1]);
      wall.castShadow = true; wall.receiveShadow = true;
      this.scene.add(wall);
    });
    window.addEventListener('resize', () => {
      this.camera.aspect = window.innerWidth / window.innerHeight;
      this.camera.updateProjectionMatrix();
      this.renderer.setSize(window.innerWidth, window.innerHeight);
    });
    window.addEventListener('keydown', e => { this.keys[e.code] = true; });
    window.addEventListener('keyup', e => { this.keys[e.code] = false; });
    this.canvas.addEventListener('mousemove', e => { this.mouse.x = e.clientX; this.mouse.y = e.clientY; this.updateMouseWorld(); });
    this.canvas.addEventListener('mousedown', e => { if (e.button === 0) this.mouse.down = true; });
    this.canvas.addEventListener('mouseup', e => { if (e.button === 0) this.mouse.down = false; });
    this.canvas.addEventListener('contextmenu', e => e.preventDefault());
  },
  updateMouseWorld() {
    const ray = new THREE.Raycaster();
    const ndc = new THREE.Vector2((this.mouse.x / window.innerWidth) * 2 - 1, -(this.mouse.y / window.innerHeight) * 2 + 1);
    ray.setFromCamera(ndc, this.camera);
    const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
    const pt = new THREE.Vector3();
    ray.ray.intersectPlane(plane, pt);
    this.mouse.worldX = pt.x; this.mouse.worldZ = pt.z;
  },
  addScore(n) { this.score += n; },
  gameOver() { this.isGameOver = true; this.isRunning = false; }
};