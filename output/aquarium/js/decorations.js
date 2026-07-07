import * as THREE from 'three';

export function createDecorations(bounds) {
  const group = new THREE.Group();

  // --- 珊瑚（递归分支结构） ---
  function createCoral(x, z, color, height) {
    const coralGroup = new THREE.Group();
    const branchMat = new THREE.MeshStandardMaterial({ color: color, roughness: 0.7 });

    function addBranch(parent, length, radius, depth) {
      if (depth <= 0 || length < 0.3) return;
      const geo = new THREE.CylinderGeometry(radius * 0.7, radius, length, 6);
      const branch = new THREE.Mesh(geo, branchMat);
      branch.position.y = length / 2;
      parent.add(branch);

      const numSub = Math.floor(Math.random() * 2) + 2;
      for (let i = 0; i < numSub; i++) {
        const sub = new THREE.Group();
        sub.position.y = length;
        sub.rotation.z = (Math.random() - 0.5) * 0.8;
        sub.rotation.x = (Math.random() - 0.5) * 0.8;
        parent.add(sub);
        addBranch(sub, length * 0.6, radius * 0.7, depth - 1);
      }
    }

    addBranch(coralGroup, height * 0.4, 0.25, 3);
    coralGroup.position.set(x, bounds.minY + 0.3, z);
    return coralGroup;
  }

  const coralColors = [0xff4466, 0xff8844, 0xff44aa, 0xee6633, 0xcc3344];
  for (let i = 0; i < 5; i++) {
    const x = THREE.MathUtils.lerp(bounds.minX + 1, bounds.maxX - 1, Math.random());
    const z = THREE.MathUtils.lerp(bounds.minZ + 1, bounds.maxZ - 1, Math.random());
    const h = 2 + Math.random() * 2;
    group.add(createCoral(x, z, coralColors[i], h));
  }

  // --- 海葵 ---
  function createAnemone(x, z, color) {
    const anemoneGroup = new THREE.Group();
    const baseGeo = new THREE.SphereGeometry(0.5, 12, 8);
    baseGeo.scale(1.5, 0.5, 1.5);
    const baseMat = new THREE.MeshStandardMaterial({ color: color, roughness: 0.8 });
    const base = new THREE.Mesh(baseGeo, baseMat);
    anemoneGroup.add(base);

    const tentacleMat = new THREE.MeshStandardMaterial({
      color: color,
      roughness: 0.6,
      emissive: color,
      emissiveIntensity: 0.1
    });
    const numTentacles = 20;
    for (let i = 0; i < numTentacles; i++) {
      const angle = (i / numTentacles) * Math.PI * 2;
      const r = 0.3 + Math.random() * 0.4;
      const tentacleGeo = new THREE.CylinderGeometry(0.04, 0.06, 1.2 + Math.random() * 0.5, 5);
      const tentacle = new THREE.Mesh(tentacleGeo, tentacleMat);
      tentacle.position.set(Math.cos(angle) * r, 0.6, Math.sin(angle) * r);
      tentacle.rotation.z = (Math.random() - 0.5) * 0.4;
      tentacle.rotation.x = (Math.random() - 0.5) * 0.4;
      tentacle.userData.baseAngle = angle;
      anemoneGroup.add(tentacle);
    }

    anemoneGroup.position.set(x, bounds.minY + 0.3, z);
    anemoneGroup.userData.isAnemone = true;
    return anemoneGroup;
  }

  const anemoneColors = [0xff99cc, 0x99ccff, 0xccff99];
  for (let i = 0; i < 3; i++) {
    const x = THREE.MathUtils.lerp(bounds.minX + 2, bounds.maxX - 2, Math.random());
    const z = THREE.MathUtils.lerp(bounds.minZ + 2, bounds.maxZ - 2, Math.random());
    group.add(createAnemone(x, z, anemoneColors[i]));
  }

  // --- 水草 ---
  function createGrass(x, z) {
    const grassGroup = new THREE.Group();
    const grassMat = new THREE.MeshStandardMaterial({
      color: 0x22aa44,
      roughness: 0.8,
      side: THREE.DoubleSide,
      transparent: true,
      opacity: 0.85
    });

    const numBlades = 8 + Math.floor(Math.random() * 6);
    for (let i = 0; i < numBlades; i++) {
      const h = 2 + Math.random() * 3;
      const bladeGeo = new THREE.PlaneGeometry(0.15, h);
      bladeGeo.translate(0, h / 2, 0);
      const blade = new THREE.Mesh(bladeGeo, grassMat);
      blade.position.set((Math.random() - 0.5) * 0.8, 0, (Math.random() - 0.5) * 0.8);
      blade.rotation.y = Math.random() * Math.PI;
      blade.userData.phase = Math.random() * Math.PI * 2;
      grassGroup.add(blade);
    }

    grassGroup.position.set(x, bounds.minY + 0.3, z);
    grassGroup.userData.isGrass = true;
    return grassGroup;
  }

  for (let i = 0; i < 4; i++) {
    const x = THREE.MathUtils.lerp(bounds.minX + 1, bounds.maxX - 1, Math.random());
    const z = THREE.MathUtils.lerp(bounds.minZ + 1, bounds.maxZ - 1, Math.random());
    group.add(createGrass(x, z));
  }

  // 缓存需要动画的装饰物
  group.userData.anemones = group.children.filter(c => c.userData.isAnemone);
  group.userData.grasses = group.children.filter(c => c.userData.isGrass);

  return group;
}

export function updateDecorations(decorations, time) {
  // 海葵触手摆动
  decorations.userData.anemones.forEach(anemone => {
    anemone.children.forEach((child, i) => {
      if (child.userData.baseAngle !== undefined) {
        child.rotation.z = Math.sin(time * 0.002 + i * 0.3) * 0.15;
        child.rotation.x = Math.cos(time * 0.002 + i * 0.3) * 0.1;
      }
    });
  });

  // 水草波浪
  decorations.userData.grasses.forEach(grass => {
    grass.children.forEach(blade => {
      if (blade.userData.phase !== undefined) {
        blade.rotation.z = Math.sin(time * 0.001 + blade.userData.phase) * 0.2;
      }
    });
  });
}