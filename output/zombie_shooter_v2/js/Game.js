// GameEngine - Core engine: scene, 2.5D top-down camera, renderer, lighting
const GameEngine = {
  scene: null,
  camera: null,
  renderer: null,
  clock: null,
  state: 'menu',
  score: 0,
  timeScale: 1.0,
  delta: 0,
  scaledDelta: 0,

  init(container) {
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x1a2a1a);
    this.scene.fog = new THREE.Fog(0x1a2a1a, 30, 80);

    this.camera = new THREE.PerspectiveCamera(50, window.innerWidth / window.innerHeight, 0.1, 200);
    this.camera.position.set(0, 25, 18);
    this.camera.lookAt(0, 0, 0);

    this.renderer = new THREE.WebGLRenderer({ antialias: true });
    this.renderer.setSize(window.innerWidth, window.innerHeight);
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    container.appendChild(this.renderer.domElement);

    const ambient = new THREE.AmbientLight(0x6080a0, 0.6);
    this.scene.add(ambient);

    const dir = new THREE.DirectionalLight(0xfff0dd, 0.8);
    dir.position.set(20, 40, 20);
    dir.castShadow = true;
    dir.shadow.mapSize.set(2048, 2048);
    dir.shadow.camera.left = -50;
    dir.shadow.camera.right = 50;
    dir.shadow.camera.top = 50;
    dir.shadow.camera.bottom = -50;
    dir.shadow.camera.near = 1;
    dir.shadow.camera.far = 100;
    this.scene.add(dir);

    const hemi = new THREE.HemisphereLight(0x80a0e0, 0x3a5a2a, 0.3);
    this.scene.add(hemi);

    this.clock = new THREE.Clock();
    window.addEventListener('resize', () => this.onResize());
  },

  onResize() {
    this.camera.aspect = window.innerWidth / window.innerHeight;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(window.innerWidth, window.innerHeight);
  },

  updateDelta() {
    this.delta = this.clock.getDelta();
    this.scaledDelta = this.delta * this.timeScale;
  },

  render() {
    this.renderer.render(this.scene, this.camera);
  },

  updateCamera(tx, tz) {
    const dx = tx - this.camera.position.x;
    const dz = (tz + 18) - this.camera.position.z;
    this.camera.position.x += dx * 0.08;
    this.camera.position.z += dz * 0.08;
    this.camera.lookAt(this.camera.position.x, 0, this.camera.position.z - 18);
  }
};