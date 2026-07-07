import * as THREE from 'three';

export function createBubbles(bounds, count = 60) {
  const group = new THREE.Group();

  const bubbleGeo = new THREE.SphereGeometry(0.08, 8, 8);
  const bubbleMat = new THREE.MeshStandardMaterial({
    color: 0xaaccff,
    transparent: true,
    opacity: 0.4,
    roughness: 0.1,
    metalness: 0.1
  });

  const bubbles = [];

  for (let i = 0; i < count; i++) {
    const bubble = new THREE.Mesh(bubbleGeo, bubbleMat);
    const size = 0.05 + Math.random() * 0.12;
    bubble.scale.set(size, size, size);

    bubble.position.set(
      THREE.MathUtils.lerp(bounds.minX, bounds.maxX, Math.random()),
      THREE.MathUtils.lerp(bounds.minY, bounds.maxY, Math.random()),
      THREE.MathUtils.lerp(bounds.minZ, bounds.maxZ, Math.random())
    );

    bubble.userData = {
      speed: 0.02 + Math.random() * 0.04,
      wobblePhase: Math.random() * Math.PI * 2,
      wobbleAmount: 0.01 + Math.random() * 0.03,
      wobbleSpeed: 0.02 + Math.random() * 0.03,
      baseX: bubble.position.x,
      baseZ: bubble.position.z
    };

    bubbles.push(bubble);
    group.add(bubble);
  }

  group.userData.bubbles = bubbles;
  group.userData.bounds = bounds;

  return group;
}

export function updateBubbles(bubbleGroup, time) {
  const bounds = bubbleGroup.userData.bounds;
  const bubbles = bubbleGroup.userData.bubbles;

  bubbles.forEach(bubble => {
    const d = bubble.userData;

    // 上升
    bubble.position.y += d.speed;

    // 左右摇摆
    bubble.position.x = d.baseX + Math.sin(time * d.wobbleSpeed + d.wobblePhase) * d.wobbleAmount;
    bubble.position.z = d.baseZ + Math.cos(time * d.wobbleSpeed + d.wobblePhase) * d.wobbleAmount;

    // 到顶重置到底部
    if (bubble.position.y > bounds.maxY) {
      bubble.position.y = bounds.minY;
      d.baseX = THREE.MathUtils.lerp(bounds.minX, bounds.maxX, Math.random());
      d.baseZ = THREE.MathUtils.lerp(bounds.minZ, bounds.maxZ, Math.random());
    }
  });
}