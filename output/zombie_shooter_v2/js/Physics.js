// PhysicsWorld - Collision detection
const PhysicsWorld = {
  obstacles: [],

  init(environment) {
    this.obstacles = [];
    if (!environment) return;
    environment.traverse(child => {
      if (child.userData && child.userData.solid && !child.userData.isWall) {
        this.obstacles.push({
          pos: new THREE.Vector3(child.position.x, 0, child.position.z),
          radius: child.userData.radius || 0.5
        });
      }
    });
  },

  checkPlayerCollision(pos, radius) {
    for (const obs of this.obstacles) {
      const dx = pos.x - obs.pos.x;
      const dz = pos.z - obs.pos.z;
      const d = Math.sqrt(dx*dx + dz*dz);
      const minDist = radius + obs.radius;
      if (d < minDist && d > 0.001) {
        const push = (minDist - d) / d;
        pos.x += dx * push;
        pos.z += dz * push;
      }
    }
    pos.x = THREE.MathUtils.clamp(pos.x, -48, 48);
    pos.z = THREE.MathUtils.clamp(pos.z, -48, 48);
  },

  checkZombieCollisions(zombies) {
    for (let i = 0; i < zombies.length; i++) {
      for (let j = i + 1; j < zombies.length; j++) {
        const a = zombies[i], b = zombies[j];
        const dx = b.pos.x - a.pos.x;
        const dz = b.pos.z - a.pos.z;
        const d = Math.sqrt(dx*dx + dz*dz);
        const minDist = a.radius + b.radius;
        if (d < minDist && d > 0.001) {
          const push = (minDist - d) * 0.5 / d;
          a.pos.x -= dx * push;
          a.pos.z -= dz * push;
          b.pos.x += dx * push;
          b.pos.z += dz * push;
        }
      }
    }
  }
};