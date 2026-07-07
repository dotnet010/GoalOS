// PlayerController - WASD movement, mouse aiming, health
const PlayerController = {
  mesh: null,
  pos: new THREE.Vector3(0, 0, 0),
  speed: 6,
  health: 100,
  maxHealth: 100,
  aimAngle: 0,
  radius: 0.4,
  keys: {},
  mouseWorld: new THREE.Vector3(0, 0, 0),
  invulnTime: 0,

  init(scene) {
    this.mesh = AssetFactory.createPlayer();
    this.mesh.position.copy(this.pos);
    scene.add(this.mesh);
    this.health = this.maxHealth;
    window.addEventListener('keydown', e => { this.keys[e.code] = true; });
    window.addEventListener('keyup', e => { this.keys[e.code] = false; });
  },

  updateMouseWorld(camera, mouseNDC) {
    const ray = new THREE.Raycaster();
    ray.setFromCamera(mouseNDC, camera);
    const plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
    ray.ray.intersectPlane(plane, this.mouseWorld);
  },

  update(delta) {
    const move = new THREE.Vector3(0, 0, 0);
    if (this.keys['KeyW']) move.z -= 1;
    if (this.keys['KeyS']) move.z += 1;
    if (this.keys['KeyA']) move.x -= 1;
    if (this.keys['KeyD']) move.x += 1;
    if (move.lengthSq() > 0) move.normalize().multiplyScalar(this.speed * delta);
    this.pos.add(move);
    this.pos.x = THREE.MathUtils.clamp(this.pos.x, -48, 48);
    this.pos.z = THREE.MathUtils.clamp(this.pos.z, -48, 48);
    this.mesh.position.copy(this.pos);

    const dx = this.mouseWorld.x - this.pos.x;
    const dz = this.mouseWorld.z - this.pos.z;
    this.aimAngle = Math.atan2(dx, dz);
    this.mesh.rotation.y = this.aimAngle;

    if (this.invulnTime > 0) {
      this.invulnTime -= delta;
      this.mesh.visible = Math.floor(this.invulnTime * 20) % 2 === 0;
    } else {
      this.mesh.visible = true;
    }
  },

  takeDamage(amount) {
    if (this.invulnTime > 0) return;
    this.health -= amount;
    this.invulnTime = 0.5;
    if (this.health < 0) this.health = 0;
  },

  reset() {
    this.pos.set(0, 0, 0);
    this.health = this.maxHealth;
    this.invulnTime = 0;
    this.mesh.visible = true;
    if (this.mesh) this.mesh.position.copy(this.pos);
  }
};